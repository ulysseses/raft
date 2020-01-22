package raft

import (
	"fmt"
	"time"

	"github.com/ulysseses/raft/raftpb"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

/*************************************************************************************************/

// ProtocolConfig configures the Raft protocol of the Raft cluster.
type ProtocolConfig struct {
	// ID of the Raft node.
	ID uint64

	// TickPeriod is the period of tiem at which the ticker should fire.
	TickPeriod time.Duration

	// HeartbeatTicks is the number of tick periods before a heartbeat
	// should fire.
	// MinElectionTicks is the minimum number of tick periods before an
	// election timeout should fire.
	// MaxElectionTicks is the maximum number of tick periods before an
	// election timeout should fire.
	HeartbeatTicks, MinElectionTicks, MaxElectionTicks uint

	// Consistency is the consistency level to use for the Raft cluster.
	Consistency Consistency

	// Lease is the duration of the read request lease. This is used only if ConsistencyLease.
	Lease time.Duration

	// HeartbeatTicker is the ticker to use for signaling when to send out heartbeats. If nil,
	// a default one based on TickPeriod and HeartbeatTicks is created and used.
	// ElectionTicker is the ticker to use for signaling when to timeout an election. If nil,
	// a default one based on TickPeriod, MinElectionTicks, and MaxElectionTicks is created and used.
	HeartbeatTicker, ElectionTicker Ticker

	// Logger, if provided, will be used to log events.
	Logger *zap.Logger

	// Debug, if true, will log events at the DEBUG verbosity/granularity.
	Debug bool
}

// Verify verifies that the configuration is correct.
func (c *ProtocolConfig) Verify() error {
	if c.ID == 0 {
		return fmt.Errorf("ID must specified and not zero")
	}
	if c.Consistency == ConsistencyLease && c.Lease <= 0 {
		return fmt.Errorf("Lease must be greater than 0 if ConsistencyLease")
	}
	if c.TickPeriod <= 0 {
		return fmt.Errorf("TickPeriod must be greater than 0")
	}
	if c.MinElectionTicks == 0 {
		return fmt.Errorf("MinElectionTicks cannot be 0")
	}
	if c.MaxElectionTicks < c.MinElectionTicks {
		return fmt.Errorf("MaxElectionTicks cannot be less than MinElectionTicks")
	}
	if c.HeartbeatTicks == 0 {
		return fmt.Errorf("HeartbeatTicks cannot be 0")
	}
	if c.HeartbeatTicks >= c.MinElectionTicks {
		return fmt.Errorf("HeartbeatTicks cannot be greater than or equal to MinElectionTicks")
	}
	return nil
}

// Build builds a ProtocolStateMachine from configuration
func (c *ProtocolConfig) Build(tr Transport) (*ProtocolStateMachine, error) {
	if err := c.Verify(); err != nil {
		return nil, err
	}

	heartbeatTicker := c.HeartbeatTicker
	electionTicker := c.ElectionTicker
	if heartbeatTicker == nil {
		heartbeatTicker = newHeartbeatTicker(c.TickPeriod, c.HeartbeatTicks)
	}
	if electionTicker == nil {
		electionTicker = newElectionTicker(c.TickPeriod, c.MinElectionTicks, c.MaxElectionTicks)
	}
	mIDs := tr.memberIDs()
	clusterSize := len(mIDs)
	members := map[uint64]*MemberState{c.ID: &MemberState{ID: c.ID}}
	for _, id := range mIDs {
		members[id] = &MemberState{ID: id}
	}

	psm := ProtocolStateMachine{
		// ticker
		heartbeatTicker: heartbeatTicker,
		electionTicker:  electionTicker,
		heartbeatC:      nil,

		// network IO
		recvChan: tr.recv(),
		sendChan: tr.send(),

		// proposals
		propReqChan:  make(chan proposalRequest),
		propRespChan: make(chan proposalResponse, 1),

		// reads
		readReqChan:  make(chan readRequest),
		readRespChan: make(chan readResponse, 1),

		// applies
		commitChan: make(chan uint64),

		// state requests
		stateReqChan:  make(chan stateReq),
		stateRespChan: make(chan State),

		// peer requests
		membersReqChan:  make(chan membersRequest),
		membersRespChan: make(chan map[uint64]MemberState),

		// raft state
		state: State{
			ID:          c.ID,
			Consistency: c.Consistency,
			QuorumSize:  clusterSize/2 + 1,
			ClusterSize: clusterSize,
			Lease:       Lease{Extension: c.Lease.Nanoseconds()},
		},
		members: members,

		log:                    newLog(),
		quorumMatchIndexBuffer: make([]uint64, clusterSize),
		nowUnixNanoFunc:        func() int64 { return time.Now().UnixNano() },
		stopChan:               make(chan struct{}, 1),

		logger: c.Logger,
		debug:  c.Debug,
	}

	return &psm, nil
}

// NewProtocolConfig builds a ProtocolConfig for a Raft node.
func NewProtocolConfig(id uint64, opts ...ProtocolConfigOption) *ProtocolConfig {
	c := protocolConfigTemplate
	c.ID = id

	var aOpt *addProtocolLogger
	for _, opt := range opts {
		if a, ok := opt.(*addProtocolLogger); ok {
			aOpt = a
		}
		opt.Transform(&c)
	}

	if c.Debug && aOpt != nil {
		aOpt.loggerCfg.Level.SetLevel(zapcore.DebugLevel)
	}

	return &c
}

var protocolConfigTemplate = ProtocolConfig{
	TickPeriod: 100 * time.Millisecond,

	// A sensible heartbeat frequency is once per 100ms.
	HeartbeatTicks: 1,

	// A sensible election timeout is 10-20x the heartbeat period.
	MinElectionTicks: 10,
	MaxElectionTicks: 20,

	Consistency: ConsistencyLease,

	// 5 heartbeat ticks. If you're feeling risky, set this to 1 election timeout.
	Lease: 500 * time.Millisecond,
}

// ProtocolConfigOption provides options to configure ProtocolConfig further.
type ProtocolConfigOption interface{ Transform(*ProtocolConfig) }

/******** WithTickPeriod ******************************************************/
type withTickPeriod struct {
	tickPeriod time.Duration
}

func (w *withTickPeriod) Transform(c *ProtocolConfig) {
	c.TickPeriod = w.tickPeriod
}

// WithTickPeriod sets a specified tick period.
func WithTickPeriod(tickPeriod time.Duration) ProtocolConfigOption {
	return &withTickPeriod{tickPeriod: tickPeriod}
}

/******** WithHeartbeatTicks **************************************************/
type withHeartbeatTicks struct {
	heartbeatTicks uint
}

func (w *withHeartbeatTicks) Transform(c *ProtocolConfig) {
	c.HeartbeatTicks = w.heartbeatTicks
}

// WithHeartbeatTicks sets the specified heartbeat ticks.
func WithHeartbeatTicks(heartbeatTicks uint) ProtocolConfigOption {
	return &withHeartbeatTicks{heartbeatTicks: heartbeatTicks}
}

/******** WithMinElectionTicks ***********************************************/
type withMinElectionTicks struct {
	minElectionTicks uint
}

func (w *withMinElectionTicks) Transform(c *ProtocolConfig) {
	c.MinElectionTicks = w.minElectionTicks
}

// WithMinElectionTicks sets the specified minimum election timeout ticks.
func WithMinElectionTicks(minElectionTicks uint) ProtocolConfigOption {
	return &withMinElectionTicks{minElectionTicks: minElectionTicks}
}

/******** WithMaxElectionTicks ***********************************************/
type withMaxElectionTicks struct {
	maxElectionTicks uint
}

func (w *withMaxElectionTicks) Transform(c *ProtocolConfig) {
	c.MaxElectionTicks = w.maxElectionTicks
}

// WithMaxElectionTicks sets the specified minimum election timeout ticks.
func WithMaxElectionTicks(maxElectionTicks uint) ProtocolConfigOption {
	return &withMaxElectionTicks{maxElectionTicks: maxElectionTicks}
}

/******** WithHeartbeatTicker ************************************************/
type withHeartbeatTicker struct {
	ticker Ticker
}

func (w *withHeartbeatTicker) Transform(c *ProtocolConfig) {
	c.HeartbeatTicker = w.ticker
}

// WithHeartbeatTicker configures to use a specified heartbeat ticker.
func WithHeartbeatTicker(ticker Ticker) ProtocolConfigOption {
	return &withHeartbeatTicker{ticker: ticker}
}

/******** WithElectionTicker ************************************************/
type withElectionTicker struct {
	ticker Ticker
}

func (w *withElectionTicker) Transform(c *ProtocolConfig) {
	c.ElectionTicker = w.ticker
}

// WithElectionTicker configures to use a specified election timeout ticker.
func WithElectionTicker(ticker Ticker) ProtocolConfigOption {
	return &withElectionTicker{ticker: ticker}
}

/******** WithConsistency ****************************************************/
type withConsistency struct {
	consistency Consistency
}

func (w *withConsistency) Transform(c *ProtocolConfig) {
	c.Consistency = w.consistency
}

// WithConsistency sets the consistency mode.
func WithConsistency(consistency Consistency) ProtocolConfigOption {
	return &withConsistency{consistency: consistency}
}

/******** WithLease **********************************************************/
type withLease struct {
	lease time.Duration
}

func (w *withLease) Transform(c *ProtocolConfig) {
	c.Lease = w.lease
}

// WithLease sets the lease duration. This should be used if using ConsistencyLease.
func WithLease(lease time.Duration) ProtocolConfigOption {
	return &withLease{lease: lease}
}

/******** AddProtocolLogger **************************************************/
type addProtocolLogger struct {
	loggerCfg zap.Config
}

func (w *addProtocolLogger) Transform(c *ProtocolConfig) {
	logger, err := w.loggerCfg.Build()
	if err != nil {
		panic(err)
	}
	c.Logger = logger.With(zap.Uint64("id", c.ID))
}

// AddProtocolLogger adds a default production zap.Logger to the configuration.
func AddProtocolLogger() ProtocolConfigOption {
	return &addProtocolLogger{
		loggerCfg: zap.NewProductionConfig(),
	}
}

/******** WithProtocolLogger **************************************************/
type withProtocolLogger struct {
	logger *zap.Logger
}

func (w *withProtocolLogger) Transform(c *ProtocolConfig) {
	c.Logger = w.logger
}

// WithProtocolLogger configures to use a specified logger for the protocol state machine.
func WithProtocolLogger(logger *zap.Logger) ProtocolConfigOption {
	return &withProtocolLogger{logger: logger}
}

/******** WithProtocolDebug ***************************************************/
type withProtocolDebug struct {
	debug bool
}

func (w *withProtocolDebug) Transform(c *ProtocolConfig) {
	c.Debug = w.debug
}

// WithProtocolDebug sets the debug field for the ProtocolConfig.
func WithProtocolDebug(debug bool) ProtocolConfigOption {
	return &withProtocolDebug{debug: debug}
}

/*************************************************************************************************/

// TransportConfig configures transport for the Raft cluster.
type TransportConfig struct {
	// ID of the Raft node to configure.
	ID uint64

	// Addresses mapping Raft node ID to address to connect to.
	Addresses map[uint64]string

	// MsgBufferSize is the max number of Raft protocol messages per peer node allowed to be buffered
	// before the Raft node can process/send them out.
	MsgBufferSize int

	// DialTimeout is the timeout for dialing to peers.
	// ReconnectDelay is the duration to wait before retrying to dial a connection.
	DialTimeout, ReconnectDelay time.Duration

	// ServerOptions is an optional list of grpc.ServerOptions to configure the gRPC server.
	ServerOptions []grpc.ServerOption

	// DialOptions is an optional list of grpc.DialOptions to configure dialing to the peer
	// gRPC servers.
	DialOptions []grpc.DialOption

	// CallOptions is an optional list of grpc.CallOptions to configure calling the Communicate RPC.
	CallOptions []grpc.CallOption

	// Logger, if provided, will be used to log events.
	Logger *zap.Logger

	// Debug, if true, will log events at the DEBUG verbosity/granularity.
	Debug bool
}

// Verify verifies that the configuration is correct.
func (c *TransportConfig) Verify() error {
	if c.ID == 0 {
		return fmt.Errorf("ID must specified and not zero")
	}
	if c.MsgBufferSize <= 0 {
		return fmt.Errorf("MsgBufferSize must be greater than 0")
	}
	if c.DialTimeout <= 0 {
		return fmt.Errorf("DialTimeout must be greater than 0")
	}
	if c.ReconnectDelay <= 0 {
		return fmt.Errorf("ReconnectDelay must be greater than 0")
	}
	return nil
}

// Build builds a Transport from configuration.
func (c *TransportConfig) Build() (Transport, error) {
	if err := c.Verify(); err != nil {
		return nil, err
	}

	if _, ok := c.Addresses[c.ID]; !ok {
		return nil, fmt.Errorf("%d is not a key into cfg.addresses", c.ID)
	}
	lis, err := listen(c.Addresses[c.ID])
	if err != nil {
		return nil, err
	}
	peers := map[uint64]*peer{}
	for id, addr := range c.Addresses {
		if id == c.ID {
			continue
		}
		pLogger := c.Logger
		if pLogger != nil {
			pLogger = pLogger.With(zap.Uint64("peer", id))
		}
		peers[id] = &peer{
			stopChan:       make(chan struct{}),
			sendChan:       make(chan raftpb.Message, c.MsgBufferSize),
			stream:         nil, // will be initialized when started
			id:             id,
			addr:           addr,
			reconnectDelay: c.ReconnectDelay,
			dialTimeout:    c.DialTimeout,
			dialOptions:    c.DialOptions,
			callOptions:    c.CallOptions,
			logger:         pLogger,
			debug:          c.Debug,
		}
	}
	t := gRPCTransport{
		lis:        lis,
		grpcServer: grpc.NewServer(c.ServerOptions...),
		id:         c.ID,
		peers:      peers,

		recvChan: make(chan raftpb.Message),
		sendChan: make(chan raftpb.Message),
		stopChan: make(chan struct{}, 2),

		logger: c.Logger,
		debug:  c.Debug,
	}
	raftpb.RegisterRaftProtocolServer(t.grpcServer, &t)
	return &t, err
}

// NewTransportConfig builds a TransportConfig for a Raft node.
func NewTransportConfig(
	id uint64,
	addresses map[uint64]string,
	opts ...TransportConfigOption,
) *TransportConfig {
	c := transportConfigTemplate
	c.ID = id
	c.Addresses = addresses

	insecure := true
	var aOpt *addTransportLogger
	for _, opt := range opts {
		if _, ok := opt.(*withSecurity); ok {
			insecure = false
		}
		if a, ok := opt.(*addTransportLogger); ok {
			aOpt = a
		}
		opt.Transform(&c)
	}

	if insecure {
		c.DialOptions = append(c.DialOptions, grpc.WithInsecure())
	}
	if c.Debug && aOpt != nil {
		aOpt.loggerCfg.Level.SetLevel(zapcore.DebugLevel)
	}

	return &c
}

// transportConfigTemplate is the default partially filled TransportConfig.
var transportConfigTemplate = TransportConfig{
	// 30 message buffer per peer client
	MsgBufferSize: 30,

	// Sensible dial timeout if the Raft election timeout is ~1-2 seconds.
	DialTimeout:    3 * time.Second,
	ReconnectDelay: 3 * time.Second,

	ServerOptions: []grpc.ServerOption{
		// Sensible keep-alive: disconnect a peer connection after ~10 seconds of inactivity.
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    5 * time.Second,
			Timeout: 5 * time.Second,
		}),
	},

	DialOptions: []grpc.DialOption{
		// Insecure by default. Use raft.WithSecurity to override.
		grpc.WithInsecure(),
		// Sensible keep-alive: disconnect from a peer's server after ~15 seconds of inactivity.
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    10 * time.Second, // minimum allowable value
			Timeout: 5 * time.Second,
		}),
	},
}

// TransportConfigOption provides options to configure TransportConfig further.
type TransportConfigOption interface{ Transform(*TransportConfig) }

/******** WithSecurity *******************************************************/
type withSecurity struct {
	opt grpc.DialOption
}

func (w *withSecurity) Transform(c *TransportConfig) {
	c.DialOptions = append(c.DialOptions, w.opt)
}

// WithSecurity configures gRPC to use security instead of the default grpc.WithInsecure option.
func WithSecurity(opt grpc.DialOption) TransportConfigOption {
	return &withSecurity{opt: opt}
}

/******** WithGRPCServerOption ***********************************************/
type withGRPCServerOption struct {
	opt grpc.ServerOption
}

func (w *withGRPCServerOption) Transform(c *TransportConfig) {
	c.ServerOptions = append(c.ServerOptions, w.opt)
}

// WithGRPCServerOption adds a grpc.ServerOption to grpc.NewServer
func WithGRPCServerOption(opt grpc.ServerOption) TransportConfigOption {
	return &withGRPCServerOption{opt: opt}
}

/******** WithGRPCDialOption *************************************************/
type withGRPCDialOption struct {
	opt grpc.DialOption
}

func (w *withGRPCDialOption) Transform(c *TransportConfig) {
	c.DialOptions = append(c.DialOptions, w.opt)
}

// WithGRPCDialOption adds a grpc.DialOption to grpc.NewServer
func WithGRPCDialOption(opt grpc.DialOption) TransportConfigOption {
	return &withGRPCDialOption{opt: opt}
}

/******** WithGRPCCallOption *************************************************/
type withGRPCCallOption struct {
	opt grpc.CallOption
}

func (w *withGRPCCallOption) Transform(c *TransportConfig) {
	c.CallOptions = append(c.CallOptions, w.opt)
}

// WithGRPCCallOption adds a grpc.CallOption to grpc.NewServer
func WithGRPCCallOption(opt grpc.CallOption) TransportConfigOption {
	return &withGRPCCallOption{opt: opt}
}

/******** AddTransportLogger *************************************************/
type addTransportLogger struct {
	loggerCfg zap.Config
}

func (w *addTransportLogger) Transform(c *TransportConfig) {
	logger, err := w.loggerCfg.Build()
	if err != nil {
		panic(err)
	}
	c.Logger = logger.With(zap.Uint64("id", c.ID))
}

// AddTransportLogger adds a default production zap.Logger to the configuration.
func AddTransportLogger() TransportConfigOption {
	return &addTransportLogger{
		loggerCfg: zap.NewProductionConfig(),
	}
}

/******** WithTransportLogger ************************************************/
type withTransportLogger struct {
	logger *zap.Logger
}

func (w *withTransportLogger) Transform(c *TransportConfig) {
	c.Logger = w.logger
}

// WithTransportLogger configures to use a specified logger for the protocol state machine.
func WithTransportLogger(logger *zap.Logger) TransportConfigOption {
	return &withTransportLogger{logger: logger}
}

/******** WithTransportDebug *************************************************/
type withTransportDebug struct {
	debug bool
}

func (w *withTransportDebug) Transform(c *TransportConfig) {
	c.Debug = w.debug
}

// WithTransportDebug sets the debug field for the TransportConfig.
func WithTransportDebug(debug bool) TransportConfigOption {
	return &withTransportDebug{debug: debug}
}

/*************************************************************************************************/

// NodeConfig configures Node-specific configuration.
type NodeConfig struct {
	// ID of the Raft node.
	ID uint64

	// Logger, if provided, will be used to log events.
	Logger *zap.Logger

	// Debug, if true, will log events at the DEBUG verbosity/granularity.
	Debug bool
}

// Verify verifies that the configuration is correct.
func (c *NodeConfig) Verify() error {
	if c.ID == 0 {
		return fmt.Errorf("ID must specified and not zero")
	}
	return nil
}

// Build builds a Raft node.
func (c *NodeConfig) Build(
	psm *ProtocolStateMachine,
	tr Transport,
	a Application,
) (*Node, error) {
	if err := c.Verify(); err != nil {
		return nil, err
	}

	n := Node{
		psm:             psm,
		tr:              tr,
		stopAppChan:     make(chan struct{}),
		stopAppErrChan:  make(chan error),
		applyFunc:       a.Apply,
		nowUnixNanoFunc: func() int64 { return time.Now().UnixNano() },
		logger:          c.Logger,
	}

	return &n, nil
}

// NewNodeConfig builds a NodeConfig for a Raft node.
func NewNodeConfig(id uint64, opts ...NodeConfigOption) *NodeConfig {
	c := NodeConfig{
		ID: id,
	}

	var aOpt *addNodeLogger
	for _, opt := range opts {
		if a, ok := opt.(*addNodeLogger); ok {
			aOpt = a
		}
		opt.Transform(&c)
	}

	if c.Debug && aOpt != nil {
		aOpt.loggerCfg.Level.SetLevel(zapcore.DebugLevel)
	}

	return &c
}

// NodeConfigOption provides options to configure Node specifically.
type NodeConfigOption interface{ Transform(*NodeConfig) }

/******** AddNodeLogger ******************************************************/
type addNodeLogger struct {
	loggerCfg zap.Config
}

func (w *addNodeLogger) Transform(c *NodeConfig) {
	logger, err := w.loggerCfg.Build()
	if err != nil {
		panic(err)
	}
	c.Logger = logger.With(zap.Uint64("id", c.ID))
}

// AddNodeLogger adds a default production zap.Logger to the configuration.
func AddNodeLogger() NodeConfigOption {
	return &addNodeLogger{
		loggerCfg: zap.NewProductionConfig(),
	}
}

/******** WithNodeLogger **************************************************/
type withNodeLogger struct {
	logger *zap.Logger
}

func (w *withNodeLogger) Transform(c *NodeConfig) {
	c.Logger = w.logger
}

// WithNodeLogger configures to use a specified logger for the protocol state machine.
func WithNodeLogger(logger *zap.Logger) NodeConfigOption {
	return &withNodeLogger{logger: logger}
}

/******** WithNodeDebug ***************************************************/
type withNodeDebug struct {
	debug bool
}

func (w *withNodeDebug) Transform(c *NodeConfig) {
	c.Debug = w.debug
}

// WithNodeDebug sets the debug field for the NodeConfig.
func WithNodeDebug(debug bool) NodeConfigOption {
	return &withNodeDebug{debug: debug}
}

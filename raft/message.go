package raft

import (
	"github.com/ulysseses/raft/raftpb"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// MsgApp
func buildApp(
	term, from, to uint64,
	commit uint64,
	entries []raftpb.Entry,
	index, logTerm uint64,
) raftpb.Message {
	return raftpb.Message{
		Term:    term,
		From:    from,
		To:      to,
		Type:    raftpb.MsgApp,
		Commit:  commit,
		Entries: entries,
		Index:   index,
		LogTerm: logTerm,
	}
}

func msgAppZapFields(msg raftpb.Message) []zapcore.Field {
	return []zapcore.Field{
		zap.String("type", msg.Type.String()),
		zap.Uint64("term", msg.Term),
		zap.Uint64("from", msg.From),
		zap.Uint64("to", msg.To),
		zap.Uint64("commit", msg.Commit),
		zap.Uint64("index", msg.Index),
		zap.Uint64("logTerm", msg.LogTerm),
	}
}

type msgApp struct {
	term    uint64
	from    uint64
	to      uint64
	commit  uint64
	index   uint64
	logTerm uint64
	entries []raftpb.Entry
}

func getApp(msg raftpb.Message) msgApp {
	return msgApp{
		term:    msg.Term,
		from:    msg.From,
		to:      msg.To,
		commit:  msg.Commit,
		index:   msg.Index,
		logTerm: msg.LogTerm,
		entries: msg.Entries,
	}
}

// MsgAppResp
func buildAppResp(term, from, to uint64, index uint64) raftpb.Message {
	return raftpb.Message{
		Term:  term,
		From:  from,
		To:    to,
		Type:  raftpb.MsgAppResp,
		Index: index,
	}
}

func msgAppRespZapFields(msg raftpb.Message) []zapcore.Field {
	return []zapcore.Field{
		zap.String("type", msg.Type.String()),
		zap.Uint64("term", msg.Term),
		zap.Uint64("from", msg.From),
		zap.Uint64("to", msg.To),
		zap.Uint64("index", msg.Index),
	}
}

type msgAppResp struct {
	term  uint64
	from  uint64
	to    uint64
	index uint64
}

func getAppResp(msg raftpb.Message) msgAppResp {
	return msgAppResp{
		term:  msg.Term,
		from:  msg.From,
		to:    msg.To,
		index: msg.Index,
	}
}

// MsgPing
func buildPing(term, from, to uint64, unixNano int64, index uint64) raftpb.Message {
	return raftpb.Message{
		Term:     term,
		From:     from,
		To:       to,
		Type:     raftpb.MsgPing,
		UnixNano: unixNano,
		Index:    index,
	}
}

func msgPingZapFields(msg raftpb.Message) []zapcore.Field {
	return []zapcore.Field{
		zap.String("type", msg.Type.String()),
		zap.Uint64("term", msg.Term),
		zap.Uint64("from", msg.From),
		zap.Uint64("to", msg.To),
		zap.Int64("unixNano", msg.UnixNano),
		zap.Uint64("index", msg.Index),
	}
}

type msgPing struct {
	term     uint64
	from     uint64
	to       uint64
	unixNano int64
	index    uint64
}

func getPing(msg raftpb.Message) msgPing {
	return msgPing{
		term:     msg.Term,
		from:     msg.From,
		to:       msg.To,
		unixNano: msg.UnixNano,
		index:    msg.Index,
	}
}

// MsgPong
func buildPong(term, from, to uint64, unixNano int64, index uint64) raftpb.Message {
	return raftpb.Message{
		Term:     term,
		From:     from,
		To:       to,
		Type:     raftpb.MsgPong,
		UnixNano: unixNano,
		Index:    index,
	}
}

func msgPongZapFields(msg raftpb.Message) []zapcore.Field {
	return []zapcore.Field{
		zap.String("type", msg.Type.String()),
		zap.Uint64("term", msg.Term),
		zap.Uint64("from", msg.From),
		zap.Uint64("to", msg.To),
		zap.Int64("unixNano", msg.UnixNano),
		zap.Uint64("index", msg.Index),
	}
}

type msgPong struct {
	term     uint64
	from     uint64
	to       uint64
	unixNano int64
	index    uint64
}

func getPong(msg raftpb.Message) msgPong {
	return msgPong{
		term:     msg.Term,
		from:     msg.From,
		to:       msg.To,
		unixNano: msg.UnixNano,
		index:    msg.Index,
	}
}

// MsgProp
func buildProp(term, from, to uint64, unixNano int64, data []byte) raftpb.Message {
	return raftpb.Message{
		Term:     term,
		From:     from,
		To:       to,
		UnixNano: unixNano,
		Entries:  []raftpb.Entry{raftpb.Entry{Data: data}},
	}
}

func msgPropZapFields(msg raftpb.Message) []zapcore.Field {
	return []zapcore.Field{
		zap.String("type", msg.Type.String()),
		zap.Uint64("term", msg.Term),
		zap.Uint64("from", msg.From),
		zap.Uint64("to", msg.To),
		zap.Int64("unixNano", msg.UnixNano),
	}
}

type msgProp struct {
	term     uint64
	from     uint64
	to       uint64
	unixNano int64
	data     []byte
}

func getProp(msg raftpb.Message) msgProp {
	return msgProp{
		term:     msg.Term,
		from:     msg.From,
		to:       msg.To,
		unixNano: msg.UnixNano,
		data:     msg.Entries[0].Data,
	}
}

// MsgPropResp
func buildPropResp(term, from, to uint64, unixNano int64, index, logTerm uint64) raftpb.Message {
	return raftpb.Message{
		Term:     term,
		From:     from,
		To:       to,
		Type:     raftpb.MsgProp,
		UnixNano: unixNano,
		Index:    index,
		LogTerm:  logTerm,
	}
}

func msgPropRespZapFields(msg raftpb.Message) []zapcore.Field {
	return []zapcore.Field{
		zap.String("type", msg.Type.String()),
		zap.Uint64("term", msg.Term),
		zap.Uint64("from", msg.From),
		zap.Uint64("to", msg.To),
		zap.Int64("unixNano", msg.UnixNano),
		zap.Uint64("index", msg.Index),
		zap.Uint64("logTerm", msg.LogTerm),
	}
}

type msgPropResp struct {
	term     uint64
	from     uint64
	to       uint64
	unixNano int64
	index    uint64
	logTerm  uint64
}

func getPropResp(msg raftpb.Message) msgPropResp {
	return msgPropResp{
		term:     msg.Term,
		from:     msg.From,
		to:       msg.To,
		unixNano: msg.UnixNano,
		index:    msg.Index,
		logTerm:  msg.LogTerm,
	}
}

// MsgVote
func buildVote(term, from, to uint64, index, logTerm uint64) raftpb.Message {
	return raftpb.Message{
		Term:    term,
		From:    from,
		To:      to,
		Type:    raftpb.MsgVote,
		Index:   index,
		LogTerm: logTerm,
	}
}

func msgVoteZapFields(msg raftpb.Message) []zapcore.Field {
	return []zapcore.Field{
		zap.String("type", msg.Type.String()),
		zap.Uint64("term", msg.Term),
		zap.Uint64("from", msg.From),
		zap.Uint64("to", msg.To),
		zap.Uint64("index", msg.Index),
		zap.Uint64("logTerm", msg.LogTerm),
	}
}

type msgVote struct {
	term    uint64
	from    uint64
	to      uint64
	index   uint64
	logTerm uint64
}

func getVote(msg raftpb.Message) msgVote {
	return msgVote{
		term:    msg.Term,
		from:    msg.From,
		to:      msg.To,
		index:   msg.Index,
		logTerm: msg.LogTerm,
	}
}

// MsgVoteResp
func buildVoteResp(term, from, to uint64) raftpb.Message {
	return raftpb.Message{
		Term: term,
		From: from,
		To:   to,
		Type: raftpb.MsgVoteResp,
	}
}

func msgVoteRespZapFields(msg raftpb.Message) []zapcore.Field {
	return []zapcore.Field{
		zap.String("type", msg.Type.String()),
		zap.Uint64("term", msg.Term),
		zap.Uint64("from", msg.From),
		zap.Uint64("to", msg.To),
	}
}

type msgVoteResp struct {
	term uint64
	from uint64
	to   uint64
}

func getVoteResp(msg raftpb.Message) msgVoteResp {
	return msgVoteResp{
		term: msg.Term,
		from: msg.From,
		to:   msg.To,
	}
}

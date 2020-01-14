package raft

import (
	"github.com/ulysseses/raft/raftpb"
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

type msgApp struct {
	term    uint64
	from    uint64
	to      uint64
	commit  uint64
	index   uint64
	logTerm uint64
	entries []raftpb.Entry
}

func (m msgApp) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("term", m.term)
	enc.AddUint64("from", m.from)
	enc.AddUint64("to", m.to)
	enc.AddUint64("commit", m.commit)
	enc.AddUint64("index", m.index)
	enc.AddUint64("logTerm", m.logTerm)
	return nil
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

type msgAppResp struct {
	term  uint64
	from  uint64
	to    uint64
	index uint64
}

func (m msgAppResp) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("term", m.term)
	enc.AddUint64("from", m.from)
	enc.AddUint64("to", m.to)
	enc.AddUint64("index", m.index)
	return nil
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

type msgPing struct {
	term     uint64
	from     uint64
	to       uint64
	unixNano int64
	index    uint64
}

func (m msgPing) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("term", m.term)
	enc.AddUint64("from", m.from)
	enc.AddUint64("to", m.to)
	enc.AddInt64("unixNano", m.unixNano)
	enc.AddUint64("index", m.index)
	return nil
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

type msgPong struct {
	term     uint64
	from     uint64
	to       uint64
	unixNano int64
	index    uint64
}

func (m msgPong) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("term", m.term)
	enc.AddUint64("from", m.from)
	enc.AddUint64("to", m.to)
	enc.AddInt64("unixNano", m.unixNano)
	enc.AddUint64("index", m.index)
	return nil
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

type msgProp struct {
	term     uint64
	from     uint64
	to       uint64
	unixNano int64
	data     []byte
}

func (m msgProp) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("term", m.term)
	enc.AddUint64("from", m.from)
	enc.AddUint64("to", m.to)
	enc.AddInt64("unixNano", m.unixNano)
	return nil
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

type msgPropResp struct {
	term     uint64
	from     uint64
	to       uint64
	unixNano int64
	index    uint64
	logTerm  uint64
}

func (m msgPropResp) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("term", m.term)
	enc.AddUint64("from", m.from)
	enc.AddUint64("to", m.to)
	enc.AddInt64("unixNano", m.unixNano)
	enc.AddUint64("index", m.index)
	enc.AddUint64("logTerm", m.logTerm)
	return nil
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

type msgVote struct {
	term    uint64
	from    uint64
	to      uint64
	index   uint64
	logTerm uint64
}

func (m msgVote) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("term", m.term)
	enc.AddUint64("from", m.from)
	enc.AddUint64("to", m.to)
	enc.AddUint64("index", m.index)
	enc.AddUint64("logTerm", m.logTerm)
	return nil
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

type msgVoteResp struct {
	term uint64
	from uint64
	to   uint64
}

func (m msgVoteResp) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("term", m.term)
	enc.AddUint64("from", m.from)
	enc.AddUint64("to", m.to)
	return nil
}

func getVoteResp(msg raftpb.Message) msgVoteResp {
	return msgVoteResp{
		term: msg.Term,
		from: msg.From,
		to:   msg.To,
	}
}

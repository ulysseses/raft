package raft

import (
	"github.com/ulysseses/raft/raftpb"
)

// MsgApp
func buildApp(
	term, from, to uint64,
	index, logTerm, commit uint64,
	entries []raftpb.Entry,
	tid int64,
	proxy uint64,
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
		Tid:     tid,
		Proxy:   proxy,
	}
}

type msgApp struct {
	term, from, to         uint64
	index, logTerm, commit uint64
	entries                []raftpb.Entry
	tid                    int64
	proxy                  uint64
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
		tid:     msg.Tid,
		proxy:   msg.Proxy,
	}
}

// MsgAppResp
func buildAppResp(
	term, from, to uint64,
	index uint64,
	tid int64,
	proxy uint64,
	success bool,
) raftpb.Message {
	return raftpb.Message{
		Term:    term,
		From:    from,
		To:      to,
		Type:    raftpb.MsgAppResp,
		Index:   index,
		Tid:     tid,
		Proxy:   proxy,
		Success: success,
	}
}

type msgAppResp struct {
	term    uint64
	from    uint64
	to      uint64
	index   uint64
	tid     int64
	proxy   uint64
	success bool
}

func getAppResp(msg raftpb.Message) msgAppResp {
	return msgAppResp{
		term:    msg.Term,
		from:    msg.From,
		to:      msg.To,
		index:   msg.Index,
		tid:     msg.Tid,
		proxy:   msg.Proxy,
		success: msg.Success,
	}
}

// MsgRead
func buildRead(term, from, to uint64, tid int64) raftpb.Message {
	return raftpb.Message{
		Term: term,
		From: from,
		To:   to,
		Type: raftpb.MsgRead,
		Tid:  tid,
	}
}

type msgRead struct {
	term uint64
	from uint64
	to   uint64
	tid  int64
}

func getRead(msg raftpb.Message) msgRead {
	return msgRead{
		term: msg.Term,
		from: msg.From,
		to:   msg.To,
		tid:  msg.Tid,
	}
}

// MsgReadResp
func buildReadResp(term, from, to uint64, tid int64, index uint64) raftpb.Message {
	return raftpb.Message{
		Term:  term,
		From:  from,
		To:    to,
		Type:  raftpb.MsgReadResp,
		Tid:   tid,
		Index: index,
	}
}

type msgReadResp struct {
	term  uint64
	from  uint64
	to    uint64
	tid   int64
	index uint64
}

func getReadResp(msg raftpb.Message) msgReadResp {
	return msgReadResp{
		term:  msg.Term,
		from:  msg.From,
		to:    msg.To,
		tid:   msg.Tid,
		index: msg.Index,
	}
}

// MsgProp
func buildProp(term, from, to uint64, tid int64, data []byte) raftpb.Message {
	return raftpb.Message{
		Term:    term,
		From:    from,
		To:      to,
		Type:    raftpb.MsgProp,
		Tid:     tid,
		Entries: []raftpb.Entry{raftpb.Entry{Data: data}},
	}
}

type msgProp struct {
	term uint64
	from uint64
	to   uint64
	tid  int64
	data []byte
}

func getProp(msg raftpb.Message) msgProp {
	return msgProp{
		term: msg.Term,
		from: msg.From,
		to:   msg.To,
		tid:  msg.Tid,
		data: msg.Entries[0].Data,
	}
}

// MsgPropResp
func buildPropResp(term, from, to uint64, tid int64, index, logTerm uint64) raftpb.Message {
	return raftpb.Message{
		Term:    term,
		From:    from,
		To:      to,
		Type:    raftpb.MsgPropResp,
		Tid:     tid,
		Index:   index,
		LogTerm: logTerm,
	}
}

type msgPropResp struct {
	term    uint64
	from    uint64
	to      uint64
	tid     int64
	index   uint64
	logTerm uint64
}

func getPropResp(msg raftpb.Message) msgPropResp {
	return msgPropResp{
		term:    msg.Term,
		from:    msg.From,
		to:      msg.To,
		tid:     msg.Tid,
		index:   msg.Index,
		logTerm: msg.LogTerm,
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

func getVoteResp(msg raftpb.Message) msgVoteResp {
	return msgVoteResp{
		term: msg.Term,
		from: msg.From,
		to:   msg.To,
	}
}

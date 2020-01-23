package raft

import (
	"github.com/ulysseses/raft/pb"
)

// MsgApp
func buildApp(
	term, from, to uint64,
	index, logTerm, commit uint64,
	entries []pb.Entry,
	tid int64,
) pb.Message {
	return pb.Message{
		Term:    term,
		From:    from,
		To:      to,
		Type:    pb.MsgApp,
		Commit:  commit,
		Entries: entries,
		Index:   index,
		LogTerm: logTerm,
		Tid:     tid,
	}
}

func buildAppRead(
	term, from, to uint64,
	commit uint64, tid int64, proxy uint64,
) pb.Message {
	return pb.Message{
		Term:   term,
		From:   from,
		To:     to,
		Type:   pb.MsgApp,
		Commit: commit,
		Tid:    tid,
		Proxy:  proxy,
	}
}

type msgApp struct {
	term, from, to         uint64
	index, logTerm, commit uint64
	entries                []pb.Entry
	tid                    int64
	proxy                  uint64
}

func getApp(msg pb.Message) msgApp {
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
	index uint64, tid int64, success bool,
) pb.Message {
	return pb.Message{
		Term:    term,
		From:    from,
		To:      to,
		Type:    pb.MsgAppResp,
		Index:   index,
		Tid:     tid,
		Success: success,
	}
}

func buildAppRespStrictRead(
	term, from, to uint64,
	tid int64, proxy uint64,
) pb.Message {
	return pb.Message{
		Term:  term,
		From:  from,
		To:    to,
		Type:  pb.MsgAppResp,
		Tid:   tid,
		Proxy: proxy,
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

func getAppResp(msg pb.Message) msgAppResp {
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
func buildRead(term, from, to uint64, tid int64) pb.Message {
	return pb.Message{
		Term: term,
		From: from,
		To:   to,
		Type: pb.MsgRead,
		Tid:  tid,
	}
}

type msgRead struct {
	term uint64
	from uint64
	to   uint64
	tid  int64
}

func getRead(msg pb.Message) msgRead {
	return msgRead{
		term: msg.Term,
		from: msg.From,
		to:   msg.To,
		tid:  msg.Tid,
	}
}

// MsgReadResp
func buildReadResp(term, from, to uint64, tid int64, index uint64) pb.Message {
	return pb.Message{
		Term:  term,
		From:  from,
		To:    to,
		Type:  pb.MsgReadResp,
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

func getReadResp(msg pb.Message) msgReadResp {
	return msgReadResp{
		term:  msg.Term,
		from:  msg.From,
		to:    msg.To,
		tid:   msg.Tid,
		index: msg.Index,
	}
}

// MsgProp
func buildProp(term, from, to uint64, tid int64, data []byte) pb.Message {
	return pb.Message{
		Term:    term,
		From:    from,
		To:      to,
		Type:    pb.MsgProp,
		Tid:     tid,
		Entries: []pb.Entry{pb.Entry{Data: data}},
	}
}

type msgProp struct {
	term uint64
	from uint64
	to   uint64
	tid  int64
	data []byte
}

func getProp(msg pb.Message) msgProp {
	return msgProp{
		term: msg.Term,
		from: msg.From,
		to:   msg.To,
		tid:  msg.Tid,
		data: msg.Entries[0].Data,
	}
}

// MsgPropResp
func buildPropResp(term, from, to uint64, tid int64, index, logTerm uint64) pb.Message {
	return pb.Message{
		Term:    term,
		From:    from,
		To:      to,
		Type:    pb.MsgPropResp,
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

func getPropResp(msg pb.Message) msgPropResp {
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
func buildVote(term, from, to uint64, index, logTerm uint64) pb.Message {
	return pb.Message{
		Term:    term,
		From:    from,
		To:      to,
		Type:    pb.MsgVote,
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

func getVote(msg pb.Message) msgVote {
	return msgVote{
		term:    msg.Term,
		from:    msg.From,
		to:      msg.To,
		index:   msg.Index,
		logTerm: msg.LogTerm,
	}
}

// MsgVoteResp
func buildVoteResp(term, from, to uint64) pb.Message {
	return pb.Message{
		Term: term,
		From: from,
		To:   to,
		Type: pb.MsgVoteResp,
	}
}

type msgVoteResp struct {
	term uint64
	from uint64
	to   uint64
}

func getVoteResp(msg pb.Message) msgVoteResp {
	return msgVoteResp{
		term: msg.Term,
		from: msg.From,
		to:   msg.To,
	}
}

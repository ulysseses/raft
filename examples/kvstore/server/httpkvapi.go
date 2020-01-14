package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

func defaultSetContext() context.Context {
	return context.Background()
}

func defaultGetContext() context.Context {
	return context.Background()
}

type httpKVAPI struct {
	kvStore       *kvStore
	logger        *zap.Logger
	sugaredLogger *zap.SugaredLogger
}

// Route routes to the correct FastHTTP handler
func (h *httpKVAPI) Route(ctx *fasthttp.RequestCtx) {
	h.logReq(ctx)
	switch string(ctx.Path()) {
	case "/store":
		if ctx.IsPut() {
			h.handleSet(ctx)
			return
		} else if ctx.IsGet() {
			h.handleGet(ctx)
			return
		}
	case "/state":
		if ctx.IsGet() {
			h.handleState(ctx)
			return
		}
	case "/peers":
		if ctx.IsGet() {
			h.handlePeers(ctx)
			return
		}
	default:
	}
	ctx.Response.Header.Set("Allow", "PUT, GET")
	ctx.Error("Method not allowed", http.StatusMethodNotAllowed)
}

func (h *httpKVAPI) handleSet(ctx *fasthttp.RequestCtx) {
	queryArgs := ctx.QueryArgs()
	k := queryArgs.Peek("k")
	v := queryArgs.Peek("v")
	err := h.kvStore.set(defaultSetContext(), string(k), string(v))
	if err != nil {
		h.logger.Error("", zap.Error(err))
	}
}

func (h *httpKVAPI) handleGet(ctx *fasthttp.RequestCtx) {
	k := string(ctx.QueryArgs().Peek("k"))
	v, ok, err := h.kvStore.get(defaultGetContext(), k)
	if err != nil {
		h.logger.Error("", zap.Error(err))
		ctx.Error(err.Error(), http.StatusInternalServerError)
		return
	}
	h.logger.Info("", zap.String("k", k), zap.String("v", v), zap.Bool("ok", ok))
	if ok {
		ctx.WriteString(v)
	} else {
		ctx.Error("Failed to GET", http.StatusNotFound)
	}
}

func (h *httpKVAPI) handleState(ctx *fasthttp.RequestCtx) {
	state := h.kvStore.node.State()
	stateStr := state.String()
	h.logger.Info("", zap.String("state", stateStr))
	ctx.WriteString(stateStr)
}

func (h *httpKVAPI) handlePeers(ctx *fasthttp.RequestCtx) {
	peers := h.kvStore.node.Peers()
	peersStr := fmt.Sprintf("%v", peers)
	h.logger.Info("", zap.String("peers", peersStr))
	ctx.WriteString(peersStr)
}

func (h *httpKVAPI) logReq(ctx *fasthttp.RequestCtx) {
	h.logger.Info(
		string(ctx.Path()),
		zap.String("method", string(ctx.Method())),
		zap.String("queryArgs", ctx.QueryArgs().String()))
}

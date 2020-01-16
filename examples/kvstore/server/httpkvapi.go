package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type httpKVAPI struct {
	kvStore                     *kvStore
	logger                      *zap.Logger
	sugaredLogger               *zap.SugaredLogger
	readTimeout, proposeTimeout time.Duration
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

func (h *httpKVAPI) handleSet(fctx *fasthttp.RequestCtx) {
	queryArgs := fctx.QueryArgs()
	k := queryArgs.Peek("k")
	v := queryArgs.Peek("v")
	ctx, cancel := context.WithTimeout(context.Background(), h.proposeTimeout)
	err := h.kvStore.set(ctx, string(k), string(v))
	cancel()
	if err != nil {
		h.logger.Error("", zap.Error(err))
	}
}

func (h *httpKVAPI) handleGet(fctx *fasthttp.RequestCtx) {
	k := string(fctx.QueryArgs().Peek("k"))
	ctx, cancel := context.WithTimeout(context.Background(), h.proposeTimeout)
	v, ok, err := h.kvStore.get(ctx, k)
	cancel()
	if err != nil {
		h.logger.Error("", zap.Error(err))
		fctx.Error(err.Error(), http.StatusInternalServerError)
		return
	}
	h.logger.Info("", zap.String("k", k), zap.String("v", v), zap.Bool("ok", ok))
	if ok {
		fctx.WriteString(v)
	} else {
		fctx.Error("Failed to GET", http.StatusNotFound)
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

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/pprofhandler"
	"go.uber.org/zap"
)

type httpKVAPI struct {
	kvStore                     *kvStore
	logger                      *zap.Logger
	readTimeout, proposeTimeout time.Duration
}

// Route routes to the correct FastHTTP handler
func (h *httpKVAPI) Route(ctx *fasthttp.RequestCtx) {
	if h.l() {
		h.logger.Info(
			string(ctx.Path()),
			zap.String("method", string(ctx.Method())),
			zap.String("queryArgs", ctx.QueryArgs().String()))
	}
	path := string(ctx.Path())
	switch path {
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
	case "/members":
		if ctx.IsGet() {
			h.handleMembers(ctx)
			return
		}
	default:
		if strings.HasPrefix(path, "/debug/pprof/") {
			pprofhandler.PprofHandler(ctx)
			return
		}
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
		if h.l() {
			h.logger.Error("", zap.Error(err))
		}
		fctx.Error(err.Error(), http.StatusInternalServerError)
	}
}

func (h *httpKVAPI) handleGet(fctx *fasthttp.RequestCtx) {
	k := string(fctx.QueryArgs().Peek("k"))
	ctx, cancel := context.WithTimeout(context.Background(), h.proposeTimeout)
	v, ok, err := h.kvStore.get(ctx, k)
	cancel()
	if err != nil {
		if h.l() {
			h.logger.Error("get failed", zap.Error(err))
		}
		fctx.Error(err.Error(), http.StatusInternalServerError)
		return
	}
	if h.l() {
		h.logger.Info("", zap.String("k", k), zap.String("v", v), zap.Bool("ok", ok))
	}
	if ok {
		fctx.WriteString(v)
	} else {
		fctx.Error("Failed to GET", http.StatusNotFound)
	}
}

func (h *httpKVAPI) handleState(ctx *fasthttp.RequestCtx) {
	state := h.kvStore.node.State()
	if b, err := json.Marshal(state); err == nil {
		ctx.Write(b)
	} else {
		if h.l() {
			h.logger.Error(
				"failed to marshal state",
				zap.Object("state", state), zap.Error(err))
		}
		ctx.Error(err.Error(), http.StatusInternalServerError)
	}
}

func (h *httpKVAPI) handleMembers(ctx *fasthttp.RequestCtx) {
	members := h.kvStore.node.Members()
	if b, err := json.Marshal(members); err == nil {
		ctx.Write(b)
	} else {
		if h.l() {
			h.logger.Error(
				"failed to marshal members",
				zap.Reflect("members", members), zap.Error(err),
			)
		}
		ctx.Error(err.Error(), http.StatusInternalServerError)
	}
}

func (h *httpKVAPI) l() bool {
	return h.logger != nil
}

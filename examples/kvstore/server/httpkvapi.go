package main

import (
	"context"
	"log"
	"net/http"

	"github.com/valyala/fasthttp"
)

func defaultSetContext() context.Context {
	return context.Background()
}

func defaultGetContext() context.Context {
	return context.Background()
}

type httpKVAPI struct {
	kvStore *kvStore
}

// Route routes to the correct FastHTTP handler
func (h *httpKVAPI) Route(ctx *fasthttp.RequestCtx) {
	switch string(ctx.Path()) {
	case "/set":
		if ctx.IsPut() {
			h.handleSet(ctx)
			return
		}
	case "/get":
		if ctx.IsGet() {
			h.handleGet(ctx)
			return
		}
	case "/state":
		if ctx.IsGet() {
			h.handleState(ctx)
			return
		}
	default:
	}
	ctx.Response.Header.Set("Allow", "PUT, GET")
	ctx.Error("Method not allowed", http.StatusMethodNotAllowed)

	if ctx.IsPut() {
		h.handleSet(ctx)
	} else if ctx.IsGet() {
		v, ok, err := h.kvStore.get(defaultGetContext(), string(k))
		if ok {
			ctx.WriteString(v)
		} else {
			if err != nil {
				log.Print(err)
				ctx.Error(err.Error(), http.StatusInternalServerError)
			} else {
				ctx.Error("Failed to GET", http.StatusNotFound)
			}
		}
	} else {
		ctx.Response.Header.Set("Allow", "PUT, GET")
		ctx.Error("Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *httpKVAPI) handleSet(ctx *fasthttp.RequestCtx) {
	queryArgs := ctx.QueryArgs()
	k := queryArgs.Peek("k")
	v := queryArgs.Peek("v")
	err := h.kvStore.set(defaultSetContext(), string(k), string(v))
	if err != nil {
		log.Print(err)
	}
}

func (h *httpKVAPI) handleGet(ctx *fasthttp.RequestCtx) {
	k := ctx.QueryArgs().Peek("k")
	v, ok, err := h.kvStore.get(defaultGetContext(), string(k))
	if ok {
		ctx.WriteString(v)
	} else {
		if err != nil {
			log.Print(err)
			ctx.Error(err.Error(), http.StatusInternalServerError)
		} else {
			ctx.Error("Failed to GET", http.StatusNotFound)
		}
	}
}

func (h *httpKVAPI) handleState(ctx *fasthttp.RequestCtx) {
	state := h.kvStore.node.State()
	ctx.WriteString(state.String())
}

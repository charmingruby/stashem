package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/charmingruby/stashem/stash"
	"github.com/gin-gonic/gin"
)

func main() {
	stash := stash.Default(
		stash.WithTTL(5*time.Second),
		stash.WithEntriesLimit(1000),
	)

	r := gin.Default()

	r.Use(rateLimiter(stash, 3))

	r.GET("/", func(ctx *gin.Context) {
		ctx.Status(http.StatusOK)
	})

	srv := &http.Server{
		Addr:              ":3000",
		Handler:           r,
		ReadHeaderTimeout: time.Second * 10,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
}

type request struct {
	IP    string `json:"ip"`
	Count int    `json:"count"`
}

func rateLimiter(st *stash.Stash, limit int) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req request

		ip, _, _ := net.SplitHostPort(ctx.Request.RemoteAddr)

		raw, err := st.Get(ip)

		if err != nil {
			req = request{IP: ip, Count: 0}
		}

		if err == nil {
			if err := json.Unmarshal(raw, &req); err != nil {
				ctx.AbortWithStatus(http.StatusInternalServerError)
				return
			}
		}

		req.Count++

		if req.Count > limit {
			ctx.AbortWithStatus(http.StatusTooManyRequests)
			return
		}

		serialized, err := json.Marshal(req)
		if err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		if err := st.Set(ip, serialized); err != nil {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		ctx.Next()
	}
}

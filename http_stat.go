package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/ggicci/httpin"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
)

const (
	StatsUrl = "/http_stat"
)

type statsRequest struct {
	Limit *uint `in:"query=limit"`
}

func handleStats(w http.ResponseWriter, r *http.Request) {

	input := r.Context().Value(httpin.Input).(*statsRequest)
	if input.Limit == nil {
		http.Error(w, "There is no LIMIT", http.StatusBadRequest)
		return
	}

	cmdStr := fmt.Sprintf(`tail -%d /var/log/nginx/edge8086.log | awk '
function format_k(num) {
    if (num >= 1000) {
        if (num %% 1000 == 0) return sprintf("%%dk", num/1000)
        else return sprintf("%%.1fk", num/1000)
    }
    return sprintf("%%d", num)
}
{
    split($1, dt, "T")
    split(dt[2], tm, "+")
    time = tm[1]
    a[time]++
    
    for (i = 1; i <= NF; i++) {
        if ($i == "in" && $(i-1) ~ /^[0-9]+$/) {
            code = $(i-1)
            if (code == 200) b[time]++
            if (code == 499) c[time]++
            if (code == 400) d[time]++
            if (code == 403) e[time]++
            if (code == 204) f[time]++
            break
        }
    }
}
END {
    printf "%%-8s| %%4s | %%4s | %%4s | %%4s | %%4s | %%4s\n", \
           strftime("%%H:%%M:%%S"), \
           "ALL", "200", "204", "499", "400", "403"
    
    for (i in a) {
        if (a[i] >= 10) {
            rps_formatted = format_k(a[i])
            printf "%%-8s| %%5s| %%4.1f%%%%| %%4.1f%%%%| %%4.1f%%%%| %%4.1f%%%%| %%4.1f%%%%\n",
                   i, rps_formatted,
                   (b[i] + 0) / a[i] * 100,
                   (f[i] + 0) / a[i] * 100,
                   (c[i] + 0) / a[i] * 100,
                   (d[i] + 0) / a[i] * 100,
                   (e[i] + 0) / a[i] * 100
        }
    }
}' | sort`, *input.Limit)

	cmd := exec.Command("sh", "-c", cmdStr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, string(output), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(output)
}

func InitStatsRoutes(httpRouter *chi.Mux) {
	httpRouter.With(
		httpin.NewInput(statsRequest{}),
	).Get(StatsUrl, func(w http.ResponseWriter, r *http.Request) {
		handleStats(w, r)
	})
}

func main() {
	router := chi.NewRouter()
	router = InitHttpRouter(router)
	InitStatsRoutes(router)

	RunHttpServer(context.Background(), router, "0.0.0.0", 8099)
}

func InitHttpRouter(httpRouter *chi.Mux) *chi.Mux {
	httpRouter.Use(middleware.Logger)
	httpRouter.Use(middleware.Recoverer)
	httpRouter.Use(middleware.Timeout(60 * time.Second))
	httpRouter.Mount("/debug", middleware.Profiler())

	httpRouter.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

	return httpRouter
}

func RunHttpServer(ctx context.Context, router *chi.Mux, host string, port uint16) {
	httpServerAddr := fmt.Sprintf("%s:%d", host, port)
	httpServer := &http.Server{
		Addr:         httpServerAddr,
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,

		// Увеличить лимиты соединений
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	errChan := make(chan error)
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

		select {
		case <-stop:
			log.Println("Shutting down gracefully...")
			httpServer.Shutdown(ctx)
		case err := <-errChan:
			log.Fatalf("Server crashed: %v", err)
		}
	}()

	log.Printf("Start listening to http://%s/", httpServerAddr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errChan <- err
		log.Fatalf("Can't start server: %v", err)
	}
}

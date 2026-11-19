package main

import (
	"errors"
	"log"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"duanjuapp/native/core"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		client := http.Client{Timeout: 2 * time.Second}
		res, err := client.Get("http://127.0.0.1:8080/healthz")
		if err != nil {
			log.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			log.Fatal(res.Status)
		}
		return
	}
	password := os.Getenv("GUOAPP_WEB_PASSWORD")
	if len(password) < 12 {
		log.Fatal("GUOAPP_WEB_PASSWORD 至少需要 12 个字符")
	}
	data := os.Getenv("GUOAPP_WEB_DATA")
	if data == "" {
		data = "data"
	}
	directory, err := filepath.Abs(data)
	if err != nil {
		log.Fatal(err)
	}
	library, err := core.NewWebLibrary(directory)
	if err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("GUOAPP_WEB_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	var proxy netip.Addr
	if value := os.Getenv("GUOAPP_WEB_TRUSTED_PROXY"); value != "" {
		proxy, err = netip.ParseAddr(value)
		if err != nil {
			log.Fatal("GUOAPP_WEB_TRUSTED_PROXY 必须是反向代理的 IP 地址")
		}
		proxy = proxy.WithZone("").Unmap()
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           newWebApp(library, password, os.Getenv("GUOAPP_WEB_SECURE_COOKIE") == "true", proxy),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	log.Printf("小果影视 Web 正在监听 %s，已加载 %d 个站源", addr, len(library.Sources()))
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

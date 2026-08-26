package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"

	"github.com/crgimenes/compterm/config"
)

type startupInfo struct {
	Version   string   `json:"version"`
	PID       int      `json:"pid"`
	Listen    string   `json:"listen"`
	URLs      []string `json:"urls"`
	Auth      bool     `json:"auth"`
	Command   string   `json:"command"`
	ConfigDir string   `json:"config_dir"`
	LogFile   string   `json:"log_file"`
}

// shareURLs returns copy-paste viewer URLs for the bound listen address. A
// wildcard host expands to the machine's hostname and unicast IPv4 addresses,
// and a configured token is embedded so the link logs the viewer in directly.
func shareURLs(listen, token string) []string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return nil
	}

	hosts := []string{host}
	ip := net.ParseIP(host)
	if host == "" || (ip != nil && ip.IsUnspecified()) {
		hosts = wildcardHosts()
	}

	query := ""
	if token != "" {
		query = "?token=" + url.QueryEscape(token)
	}

	urls := make([]string, 0, len(hosts))
	for _, h := range hosts {
		urls = append(urls, "http://"+net.JoinHostPort(h, port)+"/"+query)
	}
	return urls
}

func wildcardHosts() []string {
	var hosts []string

	name, err := os.Hostname()
	if err == nil && name != "" {
		hosts = append(hosts, name)
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		addrs = nil
	}
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.IsLoopback() {
			continue
		}
		v4 := ipn.IP.To4()
		if v4 == nil {
			continue
		}
		hosts = append(hosts, v4.String())
	}

	if len(hosts) == 0 {
		hosts = append(hosts, "localhost")
	}
	return hosts
}

// printBanner tells the operator, before the shared shell starts, where the
// session is reachable. Printed directly to stdout: it runs before the pty
// exists, so viewers never see it. With -json the same information is a single
// machine-readable line.
func printBanner(listen, logFile string) {
	cfg := config.CFG
	info := startupInfo{
		Version:   Version,
		PID:       os.Getpid(),
		Listen:    listen,
		URLs:      shareURLs(listen, cfg.AuthToken),
		Auth:      cfg.AuthToken != "",
		Command:   cfg.Command,
		ConfigDir: cfg.Path,
		LogFile:   logFile,
	}

	if cfg.JSON {
		out, err := json.Marshal(info)
		if err != nil {
			return
		}
		fmt.Println(string(out))
		return
	}

	fmt.Printf("compterm %s (pid %d)\n", info.Version, info.PID)
	fmt.Printf("sharing %s read-only, listening on %s\n", info.Command, info.Listen)
	for _, u := range info.URLs {
		fmt.Printf("  %s\n", u)
	}
	if info.Auth {
		fmt.Println("auth: token required (the links above include it)")
	} else {
		fmt.Println("auth: open, no token required")
	}
	fmt.Printf("config: %s\nlog: %s\n", info.ConfigDir, info.LogFile)
	fmt.Println("exit the shell to stop sharing")
}

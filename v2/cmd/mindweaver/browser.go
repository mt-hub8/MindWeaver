package main

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func validateBrowserLaunchURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Hostname() != "127.0.0.1" ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment == "" {
		return errors.New("browser launch URL is not the exact loopback bootstrap shape")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 || net.JoinHostPort(parsed.Hostname(), parsed.Port()) != parsed.Host {
		return errors.New("browser launch URL requires a literal loopback host and explicit port")
	}
	fragment, err := url.ParseQuery(parsed.Fragment)
	if err != nil || len(fragment) != 1 || len(fragment["bootstrap"]) != 1 || strings.TrimSpace(fragment.Get("bootstrap")) == "" {
		return errors.New("browser launch URL requires one bootstrap fragment")
	}
	return nil
}

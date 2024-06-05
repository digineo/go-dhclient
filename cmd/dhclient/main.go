package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/digineo/go-dhclient"
	"github.com/google/gopacket/layers"
)

var (
	options       = optionList{}
	requestParams = byteList{}
)

func init() {
	flag.Usage = func() {
		fmt.Printf("syntax: %s [flags] IFNAME\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Var(&options, "option", "custom DHCP option for the request (code,value)")
	flag.Var(&requestParams, "request", "Additional value for the DHCP Request List Option 55 (code)")
}

func main() {
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	ifname := flag.Arg(0)
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		fmt.Printf("unable to find interface %s: %s\n", ifname, err)
		os.Exit(1)
	}

	logHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(logHandler)

	client := dhclient.Client{
		Iface:  iface,
		Logger: logger,
		OnBound: func(lease *dhclient.Lease) {
			logger.Info("bound", "lease", lease)
		},
	}

	// Add requests for default options
	for _, param := range dhclient.DefaultParamsRequestList {
		logger.Info("Requesting default option", "param", param)
		client.AddParamRequest(layers.DHCPOpt(param))
	}

	// Add requests for custom options
	for _, param := range requestParams {
		logger.Info("Requesting custom option", "param", param)
		client.AddParamRequest(layers.DHCPOpt(param))
	}

	// Add hostname option
	hostname, _ := os.Hostname()
	client.AddOption(layers.DHCPOptHostname, []byte(hostname))

	// Add custom options
	for _, option := range options {
		slog.Info("Adding custom option", "type", option.Type, "value", fmt.Sprintf("0x%x", option.Data))
		client.AddOption(option.Type, option.Data)
	}

	client.Start()
	defer client.Stop()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1)
	for {
		sig := <-c
		logger.Info("received signal", "type", sig)
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			return
		case syscall.SIGHUP:
			logger.Info("renew lease")
			client.Renew()
		case syscall.SIGUSR1:
			logger.Info("acquire new lease")
			client.Rebind()
		}
	}
}

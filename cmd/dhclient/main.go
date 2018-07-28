package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/digineo/go-dhclient"
	"github.com/google/gopacket/layers"
)

var options = make(optionMap)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "syntax: %s [flags] IFNAME\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Var(&options, "option", "custom DHCP option (code,value)")
}

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	log.Println(options)

	hostname, _ := os.Hostname()
	ifname := flag.Arg(0)

	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		fmt.Printf("unable to find interface %s: %s\n", ifname, err)
		os.Exit(1)
	}

	dhcpOptions := []dhclient.Option{
		{Type: layers.DHCPOptHostname, Data: []byte(hostname)},
		{Type: layers.DHCPOptParamsRequest, Data: dhclient.DefaultParamsRequestList},
	}

	for k, v := range options {
		var data []byte

		if strings.HasPrefix(v, "0x") {
			data, err = hex.DecodeString(v[2:])
			if err != nil {
				fmt.Printf("value \"%s\" is invalid: %s\n", v, err)
				os.Exit(1)
			}
		} else {
			data = []byte(v)
		}

		dhcpOptions = append(dhcpOptions,
			dhclient.Option{Type: layers.DHCPOpt(k), Data: data},
		)
	}

	client := dhclient.Client{
		Iface:    iface,
		Hostname: hostname,
		OnBound: func(lease *dhclient.Lease) {
			log.Printf("Bound: %+v", lease)
		},
		DHCPOptions: dhcpOptions,
	}

	client.Start()
	defer client.Stop()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1)
	for {
		sig := <-c
		log.Println("received", sig)
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			return
		case syscall.SIGHUP:
			log.Println("renew lease")
			client.Renew()
		case syscall.SIGUSR1:
			log.Println("acquire new lease")
			client.Rebind()
		}
	}
}

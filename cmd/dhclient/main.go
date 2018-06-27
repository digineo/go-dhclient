package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/digineo/go-dhclient"
	"github.com/google/gopacket/layers"
)

type mapVar map[uint8]string

func (v *mapVar) Set(value string) error {
	i := strings.Index(value, ",")
	if i < 0 {
		return errors.New("invalid \"code,value\" pair")
	}

	code, err := strconv.Atoi(value[:i])
	if err != nil {
		return errors.New(fmt.Sprintf("option code \"%s\" is invalid", value[:i]))
	}

	value = value[i+1:]
	(*v)[uint8(code)] = value

	return nil
}

func (v *mapVar) String() string {
	return ""
}

var options = make(mapVar)

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

	hostname, _ := os.Hostname()
	ifname := flag.Arg(0)

	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		fmt.Printf("unable to find interface %s: %s\n", ifname, err)
		os.Exit(1)
	}

	dhcpOptions := []dhclient.Option{
		{layers.DHCPOptHostname, []byte(hostname)},
		{layers.DHCPOptParamsRequest, dhclient.DefaultParamsRequestList},
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
			dhclient.Option{layers.DHCPOpt(k), data},
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

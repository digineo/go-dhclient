package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/digineo/go-dhclient"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	hostname, _ := os.Hostname()
	ifname := os.Args[1]

	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		panic(err)
	}

	client := dhclient.Client{
		Iface:    iface,
		Hostname: hostname,
		OnBound: func(lease *dhclient.Lease) {
			log.Printf("Bound: %+v", lease)
		},
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

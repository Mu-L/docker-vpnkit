package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkg/errors"

	vpnkit "github.com/moby/vpnkit/go/pkg/vpnkit"
	"github.com/moby/vpnkit/go/pkg/vpnkit/transport"
	"github.com/spf13/viper"
)

// Expose ports via the control interface

var (
	controlVsock string
	controlPipe  string

	debug       bool
	interactive bool
)

func connectClient() (vpnkit.Client, error) {
	if controlPipe != "" {
		t := transport.NewUnixTransport()
		return vpnkit.NewClient(t, controlPipe)
	}

	if controlVsock != "" {
		t := transport.NewVsockTransport()
		return vpnkit.NewClient(t, controlVsock)

	}
	return nil, errors.New("Please supply either -control-vsock or -control-pipe arguments")
}

func main() {
	viper.SetConfigName("vpnkit-expose-port")
	viper.AddConfigPath("/etc/")
	viper.AddConfigPath(".")
	viper.SetDefault("control-vsock", fmt.Sprintf("%d", vpnkit.DefaultControlVsock))
	viper.SetDefault("control-pipe", "")
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("unable to read config file: %s", err)
	}

	proto := flag.String("proto", "tcp", "proxy protocol (tcp/udp/unix)")
	hostIP := flag.String("host-ip", "", "host ip")
	hostPort := flag.Int("host-port", -1, "host port")
	hostPath := flag.String("host-path", "", "host path to forward")
	containerIP := flag.String("container-ip", "", "container ip")
	containerPort := flag.Int("container-port", -1, "container port")
	containerPath := flag.String("container-path", "", "container path to forward to")
	local := flag.String("local-bind", "", "bind only on the Host, not in the VM (default: best-effort)")
	flag.BoolVar(&debug, "debug", false, "debug interaction with docker")
	flag.BoolVar(&interactive, "i", false, "print success/failure to stdout/stderr")

	defaultControlVsock, ok := viper.Get("control-vsock").(string)
	if !ok {
		log.Printf("In config file, control-vsock should be a string\n")
		os.Exit(1)
	}
	flag.StringVar(&controlVsock, "control-vsock", defaultControlVsock, "AF_VSOCK port to connect to the Host control-plane")
	defaultControlPipe, ok := viper.Get("control-pipe").(string)
	if !ok {
		log.Printf("In config file, control-pipe should be a string\n")
		os.Exit(1)
	}
	flag.StringVar(&controlPipe, "control-pipe", defaultControlPipe, "Unix domain socket or Windows named pipe to connect to the Host control-plane")

	localBind := bestEffortLocalBind // default
	// Attempt to remain backwards compatible for existing scripts which have `-no-local-ip` as a flag.
	// Note there are no existing scripts which attempt to provide a `true` or `false` argument.
	var args []string
	for _, arg := range os.Args {
		if arg == "-no-local-ip" {
			localBind = neverLocalBind
			continue
		}
		args = append(args, arg)
	}
	os.Args = args
	flag.Parse()

	// Respect the new -local-bind argument
	switch *local {
	case "":
		// default from code above
	case "best-effort":
		localBind = bestEffortLocalBind
	case "always":
		localBind = alwaysLocalBind
	case "never":
		localBind = neverLocalBind
	default:
		log.Fatal("-local-bind argument must be 'best-effort' or 'always' or 'never'")
	}

	c, err := connectClient()
	if err != nil {
		log.Fatal(err)
	}
	var p vpnkit.Port
	switch *proto {
	case "tcp", "udp":
		p = vpnkit.Port{
			Proto:   vpnkit.Protocol(*proto),
			OutIP:   net.ParseIP(*hostIP),
			OutPort: uint16(*hostPort),
			InIP:    net.ParseIP(*containerIP),
			InPort:  uint16(*containerPort),
		}
	case "unix":
		p = vpnkit.Port{
			Proto:   vpnkit.Protocol(*proto),
			OutPath: *hostPath,
			InPath:  *containerPath,
		}
	default:
		log.Fatalf("Unknown protocol %s. Use tcp, udp or unix", *proto)
	}

	maybeLocalBind(p, localBind)

	ctx := context.Background()
	if err = c.Expose(ctx, &p); err != nil {
		sendError(err)
		// never get here
	}
	defer c.Unexpose(ctx, &p)

	sendOK()

	ch := make(chan os.Signal, 10)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGSTOP)
	<-ch
}

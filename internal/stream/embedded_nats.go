package stream

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

type EmbeddedNats struct {
	Server *server.Server
	Client *nats.Conn
	Stream nats.JetStreamContext
}

var embeddedIns = &EmbeddedNats{
	Server: nil,
	Client: nil,
	Stream: nil,
}

func StartEmbeddedServer(nodeName string, isRoot bool, bindAddressLeafNode, bindAddress, rootNatsURL string) (*EmbeddedNats, error) {
	host, port, err := parseHostAndPort(bindAddress)
	if err != nil {
		return nil, err
	}

	lhost, lport, err := parseHostAndPort(bindAddressLeafNode)
	if err != nil {
		return nil, err
	}

	//TODO: Pass server options
	opts := &server.Options{
		Host:               host,
		Port:               port,
		ServerName:         nodeName,
		StoreDir:           "/",
		NoSigs:             true,
		JetStream:          true,
		JetStreamDomain:    "embedded",
		JetStreamMaxMemory: -1,
		JetStreamMaxStore:  -1,
		// Cluster: server.ClusterOpts{
		// 	Name: "c4i-sso-agent",
		// },
		LeafNode: server.LeafNodeOpts{},
	}

	if isRoot {
		opts.LeafNode.Host = lhost
		opts.LeafNode.Port = lport
	}

	if !isRoot && rootNatsURL != "" {
		//opts.DontListen = true
		opts.LeafNode.Remotes = parseRemoteLeafOpts(rootNatsURL)
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, err
	}

	ns.SetLogger(
		&natsLogger{log.With().Str("from", "nats").Logger()},
		opts.Debug,
		opts.Trace,
	)

	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		return nil, errors.New("NATS Server time out")
	}

	embeddedIns.Server = ns

	//TODO: Pass client options
	clientOpts := []nats.Option{
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(20),
		nats.ReconnectWait(3 * time.Second),
		nats.DisconnectErrHandler(func(conn *nats.Conn, err error) {
			log.Debug().Msgf("disconnected with NATs %v", err)
		}),
		nats.ReconnectHandler(func(conn *nats.Conn) {
			log.Debug().Msgf("reconnected with NATs %v", conn.ConnectedUrl())
		}),
	}

	if !isRoot && rootNatsURL != "" {
		clientOpts = append(clientOpts, nats.InProcessServer(ns))
	}

	log.Debug().Msgf("Nats URL: %s", ns.ClientURL())
	nc, err := nats.Connect(ns.ClientURL(), clientOpts...)
	if err != nil {
		return nil, err
	}

	embeddedIns.Client = nc

	//TODO: Pass JS options
	jsOpts := []nats.JSOpt{}

	js, err := nc.JetStream(jsOpts...)

	if err != nil {
		nc.Close()
		ns.Shutdown()
		return nil, err
	}
	embeddedIns.Stream = js
	return embeddedIns, nil
}

func parseHostAndPort(adr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(adr)
	if err != nil {
		return "", 0, err
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}

	return host, port, nil
}

func parseRemoteLeafOpts(rootNatsURL string) []*server.RemoteLeafOpts {
	rootURL, _ := url.Parse(rootNatsURL)

	opts := []*server.RemoteLeafOpts{
		{
			URLs: []*url.URL{rootURL},
			Hub:  true,
		},
	}

	return opts
}

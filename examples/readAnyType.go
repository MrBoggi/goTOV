package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/debug"
	"github.com/gopcua/opcua/ua"
)

func main() {
	var (
		endpoint = flag.String("endpoint", "opc.tcp://192.168.10.150:4840", "OPC UA Endpoint URL")
		nodeID   = flag.String("node", "", "NodeID to read, such as ns=5;i=123")
		user     = flag.String("user", "", "username for opcua server")
		pass     = flag.String("pass", "", "password for opcua server")
	)
	flag.BoolVar(&debug.Enable, "debug", false, "enable debug logging, meaning see all connection handshakes and stuff")
	flag.Parse()
	log.SetFlags(0)

	ctx := context.Background()

	//Get all endpoint
	endpoints, err := opcua.GetEndpoints(ctx, *endpoint)
	if err != nil {
		log.Fatal(err)
	}

	//Select an endpoint that has security policy None and security mode None
	ep, err := opcua.SelectEndpoint(endpoints, "None", ua.MessageSecurityModeFromString("None"))
	if err != nil {
		log.Fatal(err)
	}

	//Make the connection settings
	connectionOptions := []opcua.Option{
		opcua.SecurityPolicy("None"),
		opcua.SecurityModeString("None"),
		opcua.SecurityMode(ua.MessageSecurityModeNone),
		opcua.AuthUsername(*user, *pass),
		opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeUserName),
	}

	c, err := opcua.NewClient(*endpoint, connectionOptions...)
	if err != nil {
		log.Fatal(err)
	}
	if err := c.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer c.Close(ctx)

	//parse node id to see if it is based on numeric, string or guid. meaning if:
	// ns=....;i=...
	// or ns=...;s=...
	// or ns=...;g=...
	id, err := ua.ParseNodeID(*nodeID)
	if err != nil {
		log.Fatalf("invalid node id: %v", err)
	}

	//send a generic read request
	req := &ua.ReadRequest{
		MaxAge: 2000,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: id},
		},
		TimestampsToReturn: ua.TimestampsToReturnBoth,
	}

	//get response
	var resp *ua.ReadResponse
	for {
		resp, err = c.Read(ctx, req)
		if err == nil {
			break
		}

		// Following switch contains known errors that can be retried by the user.
		// Best practice is to do it on read operations.
		switch {
		case err == io.EOF && c.State() != opcua.Closed:
			// has to be retried unless user closed the connection
			time.After(1 * time.Second)
			continue

		case errors.Is(err, ua.StatusBadSessionIDInvalid):
			// Session is not activated has to be retried. Session will be recreated internally.
			time.After(1 * time.Second)
			continue

		case errors.Is(err, ua.StatusBadSessionNotActivated):
			// Session is invalid has to be retried. Session will be recreated internally.
			time.After(1 * time.Second)
			continue

		case errors.Is(err, ua.StatusBadSecureChannelIDInvalid):
			// secure channel will be recreated internally.
			time.After(1 * time.Second)
			continue

		default:
			log.Fatalf("Read failed: %s", err)
		}
	}

	if resp != nil && resp.Results[0].Status != ua.StatusOK {
		log.Fatalf("Status not OK: %v", resp.Results[0].Status)
	}

	//response is variant, as we cant know what was dataType of node; int, bool, or what...
	//this does not work for structs
	log.Printf("%#v", resp.Results[0].Value.Value())
}

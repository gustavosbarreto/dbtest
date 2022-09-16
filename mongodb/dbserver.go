package dbtest

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/gustavosbarreto/dbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"gopkg.in/tomb.v2"
)

func init() {
	cmd := exec.Command("/bin/sh", "-c", "docker info")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "---- Failed to initialize dbtest:\n")
		fmt.Fprintf(os.Stderr, out.String())
		panic("Docker is not installed or is not running properly")
	}

	dbtest.RegisterDriver("mongodb", func() dbtest.Driver { return &Driver{version: "4.4.4"} })
}

var _ dbtest.Driver = (*Driver)(nil)

type Driver struct {
	ctx     context.Context
	config  *dbtest.Config
	version string
	client  *mongo.Client
	output  bytes.Buffer
	server  *exec.Cmd
	host    string
	tomb    tomb.Tomb
}

func (dbs *Driver) SetConfig(config *dbtest.Config) {
	dbs.config = config
}

func (dbs *Driver) SetVersion(version string) {
	dbs.version = version
}

func (dbs *Driver) start() {
	if dbs.server != nil {
		panic("Driver already started")
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("unable to listen on a local address: " + err.Error())
	}

	addr := l.Addr().(*net.TCPAddr)
	l.Close()

	args := []string{
		"run", "--rm", "--net=host", fmt.Sprintf("mongo:%s", dbs.version),
		"--bind_ip", "127.0.0.1",
		"--port", strconv.Itoa(addr.Port),
		"--nojournal",
	}

	dbs.host = addr.String()
	dbs.tomb = tomb.Tomb{}
	dbs.server = exec.Command("docker", args...)
	dbs.server.Stdout = &dbs.output
	dbs.server.Stderr = &dbs.output

	if err = dbs.server.Start(); err != nil {
		// print error to facilitate troubleshooting as the panic will be caught in a panic handler
		fmt.Fprintf(os.Stderr, "mongod failed to start: %v\n", err)
		panic(err)
	}

	dbs.tomb.Go(dbs.monitor)
	dbs.Wipe()
}

func (dbs *Driver) monitor() error {
	dbs.server.Process.Wait()
	if dbs.tomb.Alive() {
		// Present some debugging information.
		fmt.Fprintf(os.Stderr, "---- mongod container died unexpectedly:\n")
		fmt.Fprintf(os.Stderr, "%s", dbs.output.Bytes())
		fmt.Fprintf(os.Stderr, "---- mongod containers running right now:\n")

		cmd := exec.Command("/bin/sh", "-c", "docker ps --filter ancestor=mongo")
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		cmd.Run()
		fmt.Fprintf(os.Stderr, "----------------------------------------\n")

		panic("mongod container died unexpectedly")
	}
	return nil
}

// Stop stops the test server process, if it is running.
//
// It's okay to call Stop multiple times. After the test server is
// stopped it cannot be restarted.
//
// All database clients must be closed before or while the Stop method
// is running. Otherwise Stop will panic after a timeout informing that
// there is a client leak.
func (dbs *Driver) Stop() {
	if dbs.client != nil {
		if dbs.client != nil {
			dbs.client.Disconnect(dbs.ctx)
			dbs.client = nil
		}
	}
	if dbs.server != nil {
		dbs.tomb.Kill(nil)

		// Windows doesn't support Interrupt
		if runtime.GOOS == "windows" {
			dbs.server.Process.Signal(os.Kill)
		} else {
			dbs.server.Process.Signal(os.Interrupt)
		}

		select {
		case <-dbs.tomb.Dead():
		case <-time.After(5 * time.Second):
			panic("timeout waiting for mongod process to die")
		}
		dbs.server = nil
	}
}

// Client returns a new client to the server. The returned client
// must be disconnected after the tests are finished.
//
// The first call to Client will start the Driver.
func (dbs *Driver) Client() interface{} {
	if dbs.server == nil {
		dbs.start()
	}

	if dbs.client == nil {
		var err error

		clientOptions := options.Client().ApplyURI("mongodb://" + dbs.host + "/test")
		clientOptions.SetConnectTimeout(dbs.config.Timeout)

		dbs.ctx = context.Background()
		dbs.client, err = mongo.Connect(dbs.ctx, clientOptions)
		if err != nil || dbs.client == nil {
			fmt.Fprintf(os.Stderr, "failed to connect to mongodb: %v\n", err)
			panic(err)
		}
	}

	return dbs.client
}

// Wipe drops all created databases and their data.
func (dbs *Driver) Wipe() {
	if dbs.server == nil || dbs.client == nil {
		return
	}

	client := dbs.Client().(*mongo.Client)

	names, err := client.ListDatabaseNames(dbs.ctx, bson.M{})
	if err != nil {
		panic(err)
	}

	for _, name := range names {
		switch name {
		case "admin", "local", "config":
		default:
			err = dbs.client.Database(name).Drop(dbs.ctx)
			if err != nil {
				panic(err)
			}
		}
	}
}

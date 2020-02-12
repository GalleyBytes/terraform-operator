# 🚇 sshtunnel

Ultra simple SSH tunnelling for Go programs.

## Installation

```bash
go get -u github.com/elliotchance/sshtunnel
```

Or better with `dep`:

```bash
dep ensure -add github.com/elliotchance/sshtunnel
```

## Example

```go
// Setup the tunnel, but do not yet start it yet.
tunnel := sshtunnel.NewSSHTunnel(
   // User and host of tunnel server, it will default to port 22
   // if not specified.
   "ec2-user@jumpbox.us-east-1.mydomain.com",

   // Pick ONE of the following authentication methods:
   sshtunnel.PrivateKeyFile("path/to/private/key.pem"), // 1. private key
   ssh.Password("password"),                            // 2. password

   // The destination host and port of the actual server.
   "dqrsdfdssdfx.us-east-1.redshift.amazonaws.com:5439",
)

// You can provide a logger for debugging, or remove this line to
// make it silent.
tunnel.Log = log.New(os.Stdout, "", log.Ldate | log.Lmicroseconds)

// Start the server in the background. You will need to wait a
// small amount of time for it to bind to the localhost port
// before you can start sending connections.
go tunnel.Start()
time.Sleep(100 * time.Millisecond)

// NewSSHTunnel will bind to a random port so that you can have
// multiple SSH tunnels available. The port is available through:
//   tunnel.Local.Port

// You can use any normal Go code to connect to the destination server
// through localhost. You may need to use 127.0.0.1 for some libraries.
//
// Here is an example of connecting to a PostgreSQL server:
conn := fmt.Sprintf("host=127.0.0.1 port=%d username=foo", tunnel.Local.Port)
db, err := sql.Open("postgres", conn)

// ...
```

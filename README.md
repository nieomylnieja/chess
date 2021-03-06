# Chess client/server over TCP sockets

This is a simple implementation of a `TCP` server/client model listening for chess clients, pairing the games and managing existing client connections and running games.

## Server

Server is written entirely in `C`. It listens for incoming connections and handles preparing (pairing) of chess matches between clients.
It recognizes four events:
- *BEGIN* with simply `"BEGIN"` as the message required for all clients starting the game
- *MOVE* with format: `<client_id>:<move>` which gets forwarded to the opponent
- *RECONNECT* with message format: `<client_id>:RECONNECT` required for the client who lost connection and wants to join back
- *END* signified by `<client_id>:END` message, which obviously means the game ended...

Server listens under `localhost:1234`. I'm to lazy to export that to `env` file, for now it's hard coded.

## Client

Client is a `golang` app building `TCP` socket communication logic with a pinch of terminal interaction (no GUI, sorry) on top of a wonderful [notnil's chess lib](https://github.com/notnil/chess).

Here's a config, all of the below listed can be set through `env` variables defined in `client.local.env`, which can be generated by running `make local-env`.
```Go
type tcpConfig struct {
	Network    string        `default:"tcp"`
	ServerHost string        `split_words:"true" default:"localhost"`
	ServerPort int           `split_words:"true" default:"1234"`
	Timeout    time.Duration `default:"30s"`
	Interval   time.Duration `default:"5s"`
	Addr       *net.TCPAddr  `ignore:"true"`
}
```

## Running

**JUST MAKE IT**

Both `server` and `client` have `Makefiles` setup and ready to go.
In both cases running:
```bash
make
```
will result in running `build` followed by `run` Makefile targets, for the cleanup run `make clean`


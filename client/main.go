package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/notnil/chess"
)

const (
	FMT         = "%s:%s"
	END_EVENT   = "END"
	BEGIN_EVENT = "BEGIN"
)

func main() {
	c := &Client{}
	c.setupTcpConnection()
	c.handshake()

	c.newGame()

	c.drawBoard()
	var move *chess.Move
	for c.gameInProgress() {
		if c.color == c.game.Position().Turn() {
			move = c.readMoveFromStdin()
		} else {
			move = c.readMoveFromSocket()
		}
		c.move(move)
		c.drawBoard()
	}
	c.send(END_EVENT)
	c.printOutcome()
}

type Client struct {
	// TCP connection related config
	config  tcpConfig
	conn    *net.TCPConn
	fdCount int
	uuid    string
	// chess related config
	color chess.Color
	game  *chess.Game
}

func (c *Client) move(move *chess.Move) {
	if err := c.game.Move(move); err != nil {
		// we'll never get here but whatever...
		log.Panicw("invalid move registered", "error", err.Error(), "invalidMove", move)
	}
}

func (c *Client) drawBoard() {
	fmt.Println(c.game.Position().Board().Draw())
}

func (c *Client) readMoveFromStdin() *chess.Move {
	// gather the new move from the user
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("It's your turn")
	fmt.Print("Type in your move: ")

	var move *chess.Move
	var moveStr string
	var err error
	// make sure we've been given a valid move and prompt the user to give us a good one
	for {
		moveStr, err = reader.ReadString('\n')
		if err != nil {
			log.Panicw("failed to read stdin", "error", err.Error())
		}
		moveStr = strings.TrimSpace(moveStr)
		move, err = chess.AlgebraicNotation{}.Decode(c.game.Position(), moveStr)
		if err != nil {
			fmt.Print("please provide a valid move using algebraic notation: ")
			continue
		}
		break
	}
	// send the move to the server, we know it's valid
	c.send(fmt.Sprintf(FMT, c.uuid, moveStr))
	return move
}

func (c *Client) readMoveFromSocket() *chess.Move {
	fmt.Println("It's your opponents turn")
	msg := c.recv()
	move, err := chess.AlgebraicNotation{}.Decode(c.game.Position(), msg)
	if err != nil {
		log.Fatalf("your malicious counterpart has sent an invalid move: %s!", msg)
	}
	fmt.Printf("%s moved to %s", c.color.Other().Name(), msg)
	return move
}

func (c *Client) printOutcome() {
	var winner chess.Color
	switch c.game.Outcome() {
	case chess.WhiteWon:
		winner = chess.White
	case chess.BlackWon:
		winner = chess.Black
	case chess.Draw:
		fmt.Println("Game ended with a draw!")
		return
	}
	if c.color == winner {
		fmt.Println("You won the game!")
	} else {
		fmt.Printf("You've lost!\n%s has won the game.\n", c.color.Other().Name())
	}
}

func (c *Client) newGame() {
	c.game = chess.NewGame(chess.UseNotation(chess.AlgebraicNotation{}))
}

func (c *Client) gameInProgress() bool {
	return c.game.Outcome() == chess.NoOutcome
}

func (c *Client) dial(noWait bool) {
	var err error
	dialer := func() error {
		c.conn, err = net.DialTCP(c.config.Network, nil, c.config.Addr)
		return err
	}
	if noWait {
		if err = dialer(); err != nil {
			log.Panicw("failed to establish connection", "error", err.Error())
		}
		c.fdCount += 2
		return
	}

	timeout := time.After(c.config.Timeout)
	tick := time.Tick(c.config.Interval)
	for {
		select {
		case <-timeout:
			log.Panicw("connection timed out", "error", err.Error())
		case <-tick:
			log.Info("trying to reconnect")
			if err = dialer(); err == nil {
				c.fdCount += 2
				log.Info("successfully connected")
				return
			}
		}
	}
}

func (c *Client) recv() string {
	p := make([]byte, 256)
	n, err := c.conn.Read(p)
	if err != nil {
		log.Panicw("failed to read response message", "error", err.Error(), "tcpConfig", c.config)
	}
	msg := string(p[:n])
	log.Debugw("successfully read response message", "msg", msg)
	return msg
}

func (c *Client) send(msg string) {
	n, err := c.conn.Write([]byte(msg))
	if err != nil {
		if n == 0 {
			c.dial(false)
		} else {
			log.Panicw("fatal write error", "error", err.Error())
		}
	}
}

func (c *Client) setupTcpConnection() {
	envconfig.MustProcess("", &c.config)
	tcpAddr, err := net.ResolveTCPAddr(c.config.Network, c.config.getAddress())
	if err != nil {
		log.Panicw("failed to resolve TCP address", "error", err.Error(), "tcpConfig", c.config)
	}
	c.config.Addr = tcpAddr
	c.dial(true)
}

func (c *Client) handshake() {
	// receive handshake response message with uuid
	c.send(BEGIN_EVENT)
	msg := c.recv()
	s := strings.Split(strings.TrimSpace(msg), ":")
	c.uuid = s[0]
	if s[1] == "WHITE" {
		c.color = chess.White
	} else {
		c.color = chess.Black
	}
	fmt.Printf("I got my uuid: %s and color: %s!\n", c.uuid, c.color.Name())
}

type tcpConfig struct {
	Network    string        `default:"tcp"`
	ServerHost string        `split_words:"true" default:"localhost"`
	ServerPort int           `split_words:"true" default:"1234"`
	Timeout    time.Duration `default:"30s"`
	Interval   time.Duration `default:"5s"`
	Addr       *net.TCPAddr  `ignore:"true"`
}

func (t tcpConfig) getAddress() string {
	return fmt.Sprintf("%s:%d", t.ServerHost, t.ServerPort)
}

func (t tcpConfig) String() string {
	return fmt.Sprintf("network: '%s', address: '%s'", t.Network, t.getAddress())
}

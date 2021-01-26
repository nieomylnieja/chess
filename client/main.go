package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/notnil/chess"
)

const (
	FMT             = "%d:%s"
	END_EVENT       = "END"
	BEGIN_EVENT     = "BEGIN"
	RECONNECT_EVENT = "RECONNECT"
)

func main() {
	welcome()
	c := NewClient()
	c.terminal.Clear()

	c.setupTcpConnection()

	c.handshake()
	c.newGame()

	var move *chess.Move
	for c.gameInProgress() {
		c.terminal.Clear()
		c.drawBoard()
		if c.color == c.game.Position().Turn() {
			move = c.readMoveFromStdin()
		} else {
			move = c.readMoveFromSocket()
		}
		c.move(move)
	}
	c.sendFmt(END_EVENT)
	c.printOutcome()
}

type terminal interface {
	Clear()
}

func NewClient() *Client {
	var t terminal
	switch runtime.GOOS {
	case "linux":
		t = LinuxTerminal{}
	case "windows":
		t = WindowsTerminal{}
	default:
		fmt.Println("unsupported operating system")
		os.Exit(1)
	}
	return &Client{terminal: t}
}

type Client struct {
	// TCP connection related config
	config  tcpConfig
	conn    *net.TCPConn
	fdCount int
	uuid    int
	// chess related config
	color    chess.Color
	game     *chess.Game
	lastMove *chess.Move
	// terminal clearing
	terminal
}

func (c *Client) move(move *chess.Move) {
	if err := c.game.Move(move); err != nil {
		// we'll never get here but whatever...
		log.Panicw("invalid move registered", "error", err.Error(), "invalidMove", move)
	}
	c.lastMove = move
}

func (c *Client) drawBoard() {
	fmt.Println(c.game.Position().Board().Draw())
}

func (c *Client) readMoveFromStdin() *chess.Move {
	// gather the new move from the user
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("It's your turn (%s)\n", c.color.Name())
	if c.lastMove != nil {
		fmt.Printf("%s's move was: %s\n", c.color.Other().Name(), c.lastMove.String())
	}
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
	c.sendFmt(moveStr)
	return move
}

func (c *Client) readMoveFromSocket() *chess.Move {
	fmt.Printf("It's your opponents turn (%s)\n", c.color.Other().Name())
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

func (c *Client) promptToContinue() bool {
	return true
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
				c.sendFmt(RECONNECT_EVENT)
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

func (c *Client) sendFmt(msg string) {
	c.send(fmt.Sprintf(FMT, c.uuid, msg))
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
	// let's just assume the server hasn't gone crazy here
	c.uuid, _ = strconv.Atoi(s[0])
	if s[1] == "WHITE" {
		c.color = chess.White
	} else {
		c.color = chess.Black
	}
	fmt.Printf("I got my uuid: %d and color: %s!\n", c.uuid, c.color.Name())
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

type LinuxTerminal struct {
}

func (t LinuxTerminal) Clear() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	_ = cmd.Run()
}

type WindowsTerminal struct {
}

func (t WindowsTerminal) Clear() {
	cmd := exec.Command("cmd", "/c", "cls")
	cmd.Stdout = os.Stdout
	_ = cmd.Run()
}

func welcome() {
	fmt.Print("Welcome to this simple chess client!\n\n")
	fmt.Print("Do you want to setup the connection details? This includes host and port address? [y/n]: ")
	s, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	s = strings.TrimSpace(s)
	if s == "y" {
		// let's just ignore those errors, If the user gives us rubbish, he won't be able to connect anyway...
		fmt.Print("Please type in host name: ")
		host, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		host = strings.TrimSpace(host)
		_ = os.Setenv("SERVER_HOST", host)
		fmt.Print("Please type in host port: ")
		port, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		port = strings.TrimSpace(port)
		_ = os.Setenv("SERVER_PORT", port)
		fmt.Printf("server address set to: %s:%s\n", host, port)
	}
	fmt.Printf(`

A few rules to keep in mind:
 - moves should be entered in algebraic notation
 - white is playing on top and black on the bottom...
 - this program is fairly simple, be ware of wild errors!

If you're ready to begin, simply press [ENTER]
`)
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Println("The server is finding a worthy opponent for you right now!")
}

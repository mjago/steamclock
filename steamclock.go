package main

import (
	"fmt"
	"github.com/blang/mpv"
	"github.com/fogleman/gg"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	cmdBuildMovie   = "/usr/bin/ffmpeg"
	clockfilePrefix = "images/clock00"
	moviePrefix     = "movies/clock_"
	mpvClientSkt    = "/tmp/mpvsocket"
	movieExt        = ".mp4"
	clockX          = 502
	clockY          = 455
)

type Clock struct {
	hourhand, minutehand, secondhand, steampunkleft, steampunkright image.Image
}

func adjHour(hour, minute int) float64 {
	fh, fm := float64((hour+6)%12), float64(minute%60)
	return gg.Radians(((fh * 30.0) - 10) + (fm * 0.5))
}

func adjMinute(minute, second int) float64 {
	fm, fs := float64((minute+28)%60), float64(second%60)
	return gg.Radians(((fm * 6.0) + 2) + (fs * 0.1))
}

func adjSecond(second int) float64 {
	fs := float64((second + 30) % 60)
	return gg.Radians((fs * 6.0) - 10)
}

func tenSecondsAhead(t time.Time) time.Time {
	return t.Add(time.Duration(10) * time.Second)
}

func printCommand(cmd *exec.Cmd) {
	fmt.Printf("==> Executing: %s\n", strings.Join(cmd.Args, " "))
}

func printError(err error) {
	if err != nil {
		os.Stderr.WriteString(fmt.Sprintf("==> Error: %s\n", err.Error()))
	}
}

func printOutput(outs []byte) {
	if len(outs) > 0 {
		fmt.Printf("==> Output: %s\n", string(outs))
	}
}

func drawClock(then time.Time, outfile string, clk Clock) {
	second, minute, hour := then.Second(), then.Minute(), then.Hour()
	ahour := adjHour(hour, minute)
	aminute := adjMinute(minute, second)
	asecond := adjSecond(second)

	var dc *gg.Context
	if second%8 < 4 {
		dc = gg.NewContextForImage(clk.steampunkleft)
	} else {
		dc = gg.NewContextForImage(clk.steampunkright)
	}

	dc.RotateAbout(aminute, clockX, clockY)
	dc.DrawImageAnchored(clk.minutehand, clockX-170, clockY, 0.0, 0.5)
	dc.RotateAbout(-aminute+ahour, clockX, clockY)
	dc.DrawImageAnchored(clk.hourhand, clockX-14, clockY, 0.0, 0.5)
	dc.RotateAbout(-ahour+asecond, clockX, clockY)
	dc.DrawImageAnchored(clk.secondhand, clockX-170, clockY, 0.0, 0.5)
	dc.SavePNG(outfile)
}

func mpvRegister() *mpv.Client {
	ll := mpv.NewIPCClient(mpvClientSkt)
	s := mpv.NewRPCServer(ll)
	rpc.Register(s)
	rpc.HandleHTTP()
	l, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Fatal("Listen error: ", err)
	}
	go http.Serve(l, nil)

	// Client
	client, err := rpc.DialHTTP("tcp", "127.0.0.1:9999")
	if err != nil {
		log.Fatal("Listen error: ", err)
	}

	rpcc := mpv.NewRPCClient(client)
	return mpv.NewClient(rpcc)
}

func buildMovieArgs(filename string) []string {
	return []string{"-start_number",
		"10", "-r", "1", "-i",
		"images/clock%04d.png",
		"-c:v", "libx264", "-vf", "fps=1", "-pix_fmt",
		"yuv420p", "-y", filename}
}

func buildMovie(filename string) {
	var waitStatus syscall.WaitStatus
	cmd := exec.Command(cmdBuildMovie, buildMovieArgs(filename)...)
	if err := cmd.Run(); err != nil {
		printError(err)
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus = exitError.Sys().(syscall.WaitStatus)
			printOutput([]byte(fmt.Sprintf("%d", waitStatus.ExitStatus())))
		}
	} else {
		waitStatus = cmd.ProcessState.Sys().(syscall.WaitStatus)
	}
}

func buildClock(then time.Time, filename string, clk Clock) {
	for count := 10; count < 10+10; count++ {
		outfile := clockfilePrefix + strconv.Itoa(count) + ".png"
		drawClock(then.Add(time.Second*time.Duration(count-10)), outfile, clk)
	}
	buildMovie(filename)
}

func displayClock(client *mpv.Client, filename string) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		printError(err)
	}
	client.Loadfile(absPath, mpv.LoadFileModeAppendPlay)
	client.SetPause(false)
}

func makeMovieName(count int) string {
	filename := moviePrefix + strconv.Itoa(count) + movieExt
	return filename
}

func loadImage(filename string) image.Image {
	var (
		img image.Image
		b   *os.File
		err error
	)
	if b, err = os.Open(filename); err != nil {
		panic("(loadImage()) failed to open " + filename)
	}
	defer b.Close()
	switch ext := filename[len(filename)-4 : len(filename)]; ext {
	case ".png":
		img, err = png.Decode(b)
	case ".jpg":
		img, err = jpeg.Decode(b)
	default:
		panic("(loadImage()) Image loading requires png or jpg format!")
	}
	if err != nil {
		panic("(loadImage()) failed to decode " + filename)
	}
	return img
}

func loadClock() Clock {
	var clk Clock
	clk.hourhand = loadImage("clock/hour.png")
	clk.minutehand = loadImage("clock/min.png")
	clk.secondhand = loadImage("clock/sec.png")
	clk.steampunkleft = loadImage("clock/left.jpg")
	clk.steampunkright = loadImage("clock/right.jpg")
	return clk
}

func initMpvClient() *mpv.Client {
	client := mpvRegister()
	client.SetFullscreen(true)
	client.SetMute(true)
	return client
}

func waitTenSecs(now, then time.Time) time.Time {
	time.Sleep(time.Millisecond * 50)
	for now.Before(then) {
		time.Sleep(time.Millisecond * 1)
		now = time.Now()
	}
	return now
}

func main() {
	var clockCount = 0
	clock := loadClock()
	now := time.Now()
	client := initMpvClient()

	for true {
		then := tenSecondsAhead(now)
		movieName := makeMovieName(clockCount)
		if clockCount += 1; clockCount > 2 {
			clockCount = 0
		}

		buildClock(then, movieName, clock)
		displayClock(client, movieName)
		now = waitTenSecs(now, then)
	}
}

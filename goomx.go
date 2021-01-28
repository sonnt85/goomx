package goomx

import (
	"os"
	"os/exec"
	"strings"

	dbus "github.com/guelfey/go.dbus"
	log "github.com/sirupsen/logrus"
	"github.com/sonnt85/gosutils/sutils"
)

const (
	envDisplay         = "DISPLAY"
	envDbusAddress     = "DBUS_SESSION_BUS_ADDRESS"
	envDbusPid         = "DBUS_SESSION_BUS_PID"
	prefixOmxDbusFiles = "/tmp/omxplayerdbus."
	suffixOmxDbusPid   = ".pid"
	pathMpris          = "/org/mpris/MediaPlayer2"
	ifaceMpris         = "org.mpris.MediaPlayer2"
	ifaceOmx           = ifaceMpris + ".omxplayer"
	exeOxmPlayer       = "omxplayer"
	keyPause           = "p"
)

var (
	user            string
	home            string
	fileOmxDbusPath string
	fileOmxDbusPid  string
)

func init() {
	SetUser(os.Getenv("USER"), os.Getenv("HOME"))
}

// SetUser sets the username (u) and home directory (h) of the user that new
// omxplayer processes will be running as. This does not change which user the
// processes will be spawned as, it is just used to find the correct D-Bus
// configuration file after a new process has been started.
func SetUser(u, h string) {
	user = u
	home = h
	fileOmxDbusPath = prefixOmxDbusFiles + user
	fileOmxDbusPid = prefixOmxDbusFiles + user + suffixOmxDbusPid
}

// New returns a new Player instance that can be used to control an OMXPlayer
// instance that is playing the video located at the specified URL.
func New(url string, args ...string) (player *Player, err error) {
	removeDbusFiles()

	cmd, err := execOmxplayer(url, args...)
	if err != nil {
		return
	}

	err = setupDbusEnvironment()
	if err != nil {
		return
	}

	conn, err := getDbusConnection()
	if err != nil {
		return
	}

	player = &Player{
		command:    cmd,
		connection: conn,
		bus:        conn.Object(ifaceOmx, pathMpris),
	}
	return
}

// getDbusConnection establishes and returns a D-Bus connection. The connection
// is made to the D-Bus service that has been set via the two `DBUS_*`
// environment variables. Since the connection's `Auth` method attempts to use
// Go's `os/user` package to get the current user's name and home directory, and
// `os/user` is not implemented for Linux-ARM, the `authMethods` parameter is
// specified explicitly rather than passing `nil`.
func getDbusConnection() (conn *dbus.Conn, err error) {
	authMethods := []dbus.Auth{
		dbus.AuthExternal(user),
		dbus.AuthCookieSha1(user, home),
	}

	log.Debug("omxplayer: opening dbus session")
	if conn, err = dbus.SessionBusPrivate(); err != nil {
		return
	}

	log.Debug("omxplayer: authenticating dbus session")
	if err = conn.Auth(authMethods); err != nil {
		return
	}

	log.Debug("omxplayer: initializing dbus session")
	err = conn.Hello()
	return
}

// setupDbusEnvironment sets the environment variables that are necessary to
// establish a D-Bus connection. If the connection's path or PID cannot be read,
// the associated error is returned.
func setupDbusEnvironment() (err error) {
	log.Debug("omxplayer: setting up dbus environment")

	path, err := getDbusPath()
	if err != nil {
		return
	}

	pid, err := getDbusPid()
	if err != nil {
		return
	}

	os.Setenv(envDisplay, ":0")
	os.Setenv(envDbusAddress, path)
	os.Setenv(envDbusPid, pid)
	return
}

// getDbusPath reads the D-Bus path from the file OMXPlayer writes it's path to.
// If the file cannot be read, it returns an error, otherwise it returns the
// path as a string.
func getDbusPath() (string, error) {
	if err := sutils.FileWaitForFileExist(fileOmxDbusPath, 500); err != nil {
		return "", err
	}
	return sutils.FileWaitContentsAndRead(fileOmxDbusPath, 500)
}

// getDbusPath reads the D-Bus PID from the file OMXPlayer writes it's PID to.
// If the file cannot be read, it returns an error, otherwise it returns the
// PID as a string.
func getDbusPid() (string, error) {
	if err := sutils.FileWaitForFileExist(fileOmxDbusPid, 500); err != nil {
		return "", err
	}
	return sutils.FileWaitContentsAndRead(fileOmxDbusPid, 500)
}

// removeDbusFiles removes the files that OMXPlayer creates containing the D-Bus
// path and PID. This ensures that when the path and PID are read in, the new
// files are read instead of the old ones.
func removeDbusFiles() {
	sutils.FileremoveFile(fileOmxDbusPath)
	sutils.FileremoveFile(fileOmxDbusPid)
}

// execOmxplayer starts a new OMXPlayer process and tells it to pause the video
// by passing a "p" on standard input.
func execOmxplayer(url string, args ...string) (cmd *exec.Cmd, err error) {
	log.Debug("omxplayer: starting omxplayer process")

	args = append(args, url)

	cmd = exec.Command(exeOxmPlayer, args...)
	cmd.Stdin = strings.NewReader(keyPause)
	err = cmd.Start()
	return
}
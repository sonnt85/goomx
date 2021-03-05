// +build !windows
package goomx

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	dbus "github.com/godbus/dbus"

	//	dbus "github.com/guelfey/go.dbus"
	log "github.com/sirupsen/logrus"
	"github.com/sonnt85/gosutils/sutils"
)

const (
	envDisplay                     = "DISPLAY"
	envDbusAddress                 = "DBUS_SESSION_BUS_ADDRESS"
	envDbusPid                     = "DBUS_SESSION_BUS_PID"
	prefixOmxDbusFiles             = "/tmp/omxplayerdbus."
	suffixOmxDbusPid               = ".pid"
	pathMpris                      = "/org/mpris/MediaPlayer2"
	ifaceMpris                     = "org.mpris.MediaPlayer2"
	ifaceOmx                       = ifaceMpris + ".omxplayer"
	exeOxmPlayer                   = "omxplayer"
	keyPause                       = "p"
	keyQuit                        = "q"
	ACTION_DECREASE_SPEED          = 1
	ACTION_INCREASE_SPEED          = 2
	ACTION_REWIND                  = 3
	ACTION_FAST_FORWARD            = 4
	ACTION_SHOW_INFO               = 5
	ACTION_PREVIOUS_AUDIO          = 6
	ACTION_NEXT_AUDIO              = 7
	ACTION_PREVIOUS_CHAPTER        = 8
	ACTION_NEXT_CHAPTER            = 9
	ACTION_PREVIOUS_SUBTITLE       = 10
	ACTION_NEXT_SUBTITLE           = 11
	ACTION_TOGGLE_SUBTITLE         = 12
	ACTION_DECREASE_SUBTITLE_DELAY = 13
	ACTION_INCREASE_SUBTITLE_DELAY = 14
	ACTION_EXIT                    = 15
	ACTION_PLAYPAUSE               = 16
	ACTION_DECREASE_VOLUME         = 17
	ACTION_INCREASE_VOLUME         = 18
	ACTION_SEEK_BACK_SMALL         = 19
	ACTION_SEEK_FORWARD_SMALL      = 20
	ACTION_SEEK_BACK_LARGE         = 21
	ACTION_SEEK_FORWARD_LARGE      = 22
	ACTION_SEEK_RELATIVE           = 25
	ACTION_SEEK_ABSOLUTE           = 26
	ACTION_STEP                    = 23
	ACTION_BLANK                   = 24
	ACTION_MOVE_VIDEO              = 27
	ACTION_HIDE_VIDEO              = 28
	ACTION_UNHIDE_VIDEO            = 29
	ACTION_HIDE_SUBTITLES          = 30
	ACTION_SHOW_SUBTITLES          = 31
	ACTION_SET_ALPHA               = 32
	ACTION_SET_ASPECT_MODE         = 33
	ACTION_CROP_VIDEO              = 34
	ACTION_PAUSE                   = 35
	ACTION_PLAY                    = 36
	ACTION_CHANGE_FILE             = 37
	ACTION_SET_LAYER               = 38
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
	if len(url) != 0 {
		cmd, err := execOmxplayer(url, args...)
		if err != nil {
			return nil, err
		}

		err = setupDbusEnvironment()
		if err != nil {
			return nil, err
		}

		conn, err := getDbusConnection()
		if err != nil {
			return nil, err
		}

		player = &Player{
			command:    cmd,
			connection: conn,
			bus:        conn.Object(ifaceOmx, pathMpris).(*dbus.Object),
		}
	} else {
		player = &Player{}
	}

	player.playlist = make([]string, 0)
	SetUser(sutils.SysGetUsername(), sutils.GetHomeDir())
	player.videodir = sutils.GetHomeDir() + "Videos/"
	os.MkdirAll(player.videodir, 0700)
	player.activePlaylist = false
	player.indexRunning = 0
	player.currentVolume = 0.03
	player.mutex = new(sync.Mutex)
	player.argsOmx = args
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

func dbusIsRunning() bool {
	var pid int
	if _pid, err := getDbusPid(); err != nil {
		return false
	} else {
		if pid, err = strconv.Atoi(_pid); err != nil {
			return false
		}
	}
	return sutils.IsProcessAlive(pid)
}

// removeDbusFiles removes the files that OMXPlayer creates containing the D-Bus
// path and PID. This ensures that when the path and PID are read in, the new
// files are read instead of the old ones.
func removeDbusFiles() {
	if !dbusIsRunning() {
		sutils.FileremoveFile(fileOmxDbusPath)
		sutils.FileremoveFile(fileOmxDbusPid)
	}
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

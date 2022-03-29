//// +build linux,arm

package goomx

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	dbus "github.com/godbus/dbus"
	"github.com/sonnt85/goring"
	"github.com/sonnt85/gosutils/sutils"
	"github.com/sonnt85/gosyncutils"
)

const (
	ifaceProps     = "org.freedesktop.DBus.Properties"
	ifaceOmxRoot   = ifaceMpris
	ifaceOmxPlayer = ifaceOmxRoot + ".Player"

	cmdQuit                 = ifaceOmxRoot + ".Quit"
	propCanQuit             = ifaceProps + ".CanQuit"
	propFullscreen          = ifaceProps + ".Fullscreen"
	propCanSetFullscreen    = ifaceProps + ".CanSetFullscreen"
	propCanRaise            = ifaceProps + ".CanRaise"
	propHasTrackList        = ifaceProps + ".HasTrackList"
	propIdentity            = ifaceProps + ".Identity"
	propSupportedURISchemes = ifaceProps + ".SupportedUriSchemes"
	propSupportedMimeTypes  = ifaceProps + ".SupportedMimeTypes"
	propCanGoNext           = ifaceProps + ".CanGoNext"
	propCanGoPrevious       = ifaceProps + ".CanGoPrevious"
	propCanSeek             = ifaceProps + ".CanSeek"
	propCanControl          = ifaceProps + ".CanControl"
	propCanPlay             = ifaceProps + ".CanPlay"
	propCanPause            = ifaceProps + ".CanPause"
	cmdNext                 = ifaceOmxPlayer + ".Next"
	cmdPrevious             = ifaceOmxPlayer + ".Previous"
	cmdPause                = ifaceOmxPlayer + ".Pause"
	cmdPlay                 = ifaceOmxPlayer + ".Play"
	cmdPlayPause            = ifaceOmxPlayer + ".PlayPause"
	cmdStop                 = ifaceOmxPlayer + ".Stop"
	cmdSeek                 = ifaceOmxPlayer + ".Seek"
	cmdSetPosition          = ifaceOmxPlayer + ".SetPosition"
	propPlaybackStatus      = ifaceProps + ".PlaybackStatus"
	cmdVolume               = ifaceProps + ".Volume"
	cmdMute                 = ifaceProps + ".Mute"
	cmdUnmute               = ifaceProps + ".Unmute"
	propPosition            = ifaceProps + ".Position"
	propAspect              = ifaceProps + ".Aspect"
	propVideoStreamCount    = ifaceProps + ".VideoStreamCount"
	propResWidth            = ifaceProps + ".ResWidth"
	propResHeight           = ifaceProps + ".ResHeight"
	propDuration            = ifaceProps + ".Duration"
	propMinimumRate         = ifaceProps + ".MinimumRate"
	propMaximumRate         = ifaceProps + ".MaximumRate"
	cmdListSubtitles        = ifaceOmxPlayer + ".ListSubtitles"
	cmdHideVideo            = ifaceOmxPlayer + ".HideVideo"
	cmdUnHideVideo          = ifaceOmxPlayer + ".UnHideVideo"
	cmdListAudio            = ifaceOmxPlayer + ".ListAudio"
	cmdListVideo            = ifaceOmxPlayer + ".ListVideo"
	cmdSelectSubtitle       = ifaceOmxPlayer + ".SelectSubtitle"
	cmdSelectAudio          = ifaceOmxPlayer + ".SelectAudio"
	cmdShowSubtitles        = ifaceOmxPlayer + ".ShowSubtitles"
	cmdHideSubtitles        = ifaceOmxPlayer + ".HideSubtitles"
	cmdAction               = ifaceOmxPlayer + ".Action"
	cmdGetSource            = ifaceOmxPlayer + ".GetSource"
	cmdOpenUri              = ifaceOmxPlayer + ".OpenUri"
	cmdRaise                = ifaceProps + ".Raise"
)

// The Player struct provides access to all of omxplayer's D-Bus methods.
type CommonOmx struct {
	command *exec.Cmd
}

type FilePlay struct {
	pathFile     string
	isStreamLink bool
	args         []string
}
type Player struct {
	command       *exec.Cmd
	bus           dbus.BusObject
	argsOmx       []string
	currentVolume float64
	*goring.Playlist[string]
	startedViewPicture       bool
	playingFile              chan FilePlay
	enablePlay               *gosyncutils.MutipleWait[bool]
	condStop                 *gosyncutils.MutipleWait[bool]
	condStartViewPicture     *gosyncutils.MutipleWait[bool]
	condStopViewPicture      *gosyncutils.MutipleWait[bool]
	condFinishCurrentPlaying *gosyncutils.MutipleWait[bool]

	condStart  *gosyncutils.MutipleWait[bool]
	ctx        context.Context
	CancelFunc context.CancelFunc
}

var Gplayer *Player

func (p *Player) PlayNextVideo() (retfile string, ok bool) {
	if p.Length() != 0 && p.enablePlay.Get() {
		if p.IsRunning() {
			p.condStop.SetThenSendBroadcast(true) // stop to play next video
		}
		retfile, err := p.Current()
		return retfile, err == nil
	} else {
		return
	}
}

func (p *Player) PlayPrevVideo() (retfile string, ok bool) {
	if p.Length() != 0 && p.enablePlay.Get() {
		retfile, _ = p.Prev()
		if p.IsRunning() {
			p.condStop.SetThenSendBroadcast(true) // stop to play next video
		}
		return retfile, true
	} else {
		return
	}
}

func (p *Player) GetPlaying() (retfile string, ok bool) {
	if p.Length() != 0 && p.IsRunning() {
		retfile, err := p.Current()
		return retfile, err == nil
	} else {
		return
	}
}

func (p *Player) AddVideoToPlaylist(finename string, index int) bool {
	return nil == p.Insert(index, finename)
}

func (p *Player) RemoveVideoFromPlaylist(index int) bool {
	return nil == p.Remove(index)
}

func (p *Player) GetPlaylistWithoutPath() []string {
	retstrs, _ := p.Copy()
	names := make([]string, len(retstrs))
	for i, v := range retstrs {
		names[i] = filepath.Base(v)
	}
	return names
}

func (p *Player) GetPlaylist() (retstrs []string) {
	retstrs, _ = p.Copy()
	return retstrs
}

func (p *Player) Stop() {
	p.enablePlay.Set(false)
	p.condStop.SetThenSendBroadcast(true) //stop if it is playing
}

func (p *Player) Play() bool {
	// if p.Length() != 0 {
	if !p.enablePlay.Get() {
		p.enablePlay.Set(true)
		p.enablePlay.Broadcast()
	}
	return true
	// } else {
	// return false
	// }
}

func (p *Player) PlayIsActive() bool {
	return p.enablePlay.Get()
}

func (p *Player) GetSavedVolume() float64 {
	return p.currentVolume
}

func (p *Player) ConfigureNewPlaylist(list []string) (chaged bool) {
	chaged = p.UpdateNewPlaylist(list)
	if chaged && p.IsRunning() { // reset play new playlist if playing
		p.condStop.SetThenSendBroadcast(true)
	}
	return
}

func (p *Player) ActiveViewDefaultPictures(picspath string) {
	if p.startedViewPicture {
		return
	}
	if sutils.PathIsExist(picspath) {
		//		fb, err := gofb.Open("/dev/fb0")
		//		if err != nil {
		//			fmt.Println("Can not open fb0", err)
		//			return
		//		}
		//		defer fb.Close()
		//		//Here is a simple example that clears the whole screen to a dark magenta:
		//		magenta := image.NewUniform(color.RGBA{255, 0, 128, 255})
		//		draw.Draw(fb, fb.Bounds(), magenta, image.ZP, draw.Src)
		//		return
		go func() {
			var cmd *exec.Cmd
			var err error
			p.startedViewPicture = true
			for {
				p.condStartViewPicture.TestThenWaitSignalIfNotMatch(true)
				p.condStartViewPicture.Set(false)

				if sutils.PathIsDir(picspath) {
					cmd = exec.Command("omxiv", "-t", "3", "-a", "center", "--transition", "blend", "--duration", "3000", picspath)
				} else if sutils.PathIsFile(picspath) {
					cmd = exec.Command("omxiv", "-a", "center", "--transition", "blend", "--duration", "3000", picspath)
				} else {
					p.startedViewPicture = false
					return
				}
				cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
				cmd.Stdout = nil
				cmd.Stderr = nil

				if err = cmd.Start(); err != nil {
					time.Sleep(time.Second * 1)
					continue
				}

				p.condStopViewPicture.TestThenWaitSignalIfNotMatch(true)
				p.condStopViewPicture.Set(false)
				if cmd.ProcessState == nil && cmd.Process != nil { //still runnning
					// syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					cmd.Process.Signal(syscall.SIGKILL)
					// fmt.Println("Release view picture")
					// cmd.Process.Kill()
				}

				cmd.Wait()
			}
		}()
	}
}

func (p *Player) __queueService() {
	var filePlay FilePlay
	var nextFile string
	// time.Sleep(time.Millisecond*100)
	p.condStartViewPicture.SetThenSendSignal(true)
	p.NextWait()
	nextFile, _ = p.PrevWait()
	filePlay.pathFile = nextFile
	p.enablePlay.TestThenWaitSignalIfNotMatch(true)
	for {
		p.playingFile <- filePlay
		time.Sleep(time.Millisecond * 2)
		for !p.condFinishCurrentPlaying.Get() {
			time.Sleep(time.Millisecond * 10)
		}
		p.condFinishCurrentPlaying.Set(false)
		p.condFinishCurrentPlaying.WaitSignal()
		p.enablePlay.TestThenWaitSignalIfNotMatch(true)
		nextFile, _ = p.NextWait()
		filePlay.pathFile = nextFile
	}
}

func (p *Player) __startService() {
	var filePlay FilePlay
	var args []string
	var err error
	fmt.Println("Waiting for play")
	go p.__queueService()
	for {
		p.condFinishCurrentPlaying.Broadcast()
		filePlay = <-p.playingFile
		p.condFinishCurrentPlaying.Set(true)
		fmt.Println("New file for play: ", filePlay.pathFile)
		if !filePlay.isStreamLink && !sutils.PathIsFile(filePlay.pathFile) {
			continue
		}
		if len(p.argsOmx) != 0 {
			args = p.argsOmx
		} else {
			args = make([]string, 0)
		}

		if len(filePlay.args) != 0 {
			args = append(args, filePlay.args...)
		}
		args = append(args, filePlay.pathFile)

		p.command = exec.Command(exeOxmPlayer, args...)
		p.command.Stdin = nil
		p.command.Stdout = nil
		p.command.Stderr = nil
		p.command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		err = p.command.Start()
		if err != nil {
			fmt.Printf("Can not start: %s - %s\n", filePlay.pathFile, err.Error())
			continue
		}
		p.condStopViewPicture.SetThenSendSignal(true)
		killcmd := func() {
			// if p.command.ProcessState == nil && p.command.Process != nil {
			if p.command.Process != nil {
				p.CmdQuit()
				syscall.Kill(-p.command.Process.Pid, syscall.SIGKILL)
				// p.command.Process.Signal(syscall.SIGKILL) //force kill process
			}
		}
		func() { // release dbus connection if exits
			endFunc := false
			defer func() {
				if !endFunc {
					killcmd()
					p.command.Wait()
				} //force kill process
				p.condStartViewPicture.SetThenSendSignal(true)
				p.condStop.Set(false) //clear signal send by controler
				p.condStart.Set(false)
			}()

			err = setupDbusEnvironment() //wait timeout dbus then set enroviment dbus
			if err != nil {
				fmt.Println("can not setupDbusEnvironment")
				return
			}

			conn, err := getDbusConnection()
			if err != nil {
				fmt.Println("can not get setupDbusEnvironment")
				return
			}
			p.bus = conn.Object(ifaceOmx, pathMpris)

			ctx, cancleFunc := context.WithCancel(context.Background())
			go func(ctx context.Context) {
				select {
				case <-p.condStop.TestThenWaitSignalIfMatch(false, true): //force kill
					killcmd()
				case <-ctx.Done():
					p.condStop.Signal()
				}
				if conn.Connected() {
					fmt.Println("Release dbus connection")
					conn.Close()
				}
			}(ctx)
			go func() {
				p.WaitForReadyWithTimeOut(time.Second * 3)
				if _, err = p.CmdVolume(p.GetSavedVolume()); err != nil {
					fmt.Println("Can not Set Volume", err)
				}
				// continue
			}()
			p.condStart.SetThenSendBroadcast(true)
			err = p.command.Wait() // wait for end video
			cancleFunc()
			endFunc = true
			fmt.Println("Finish play ", filePlay.pathFile)
		}()
	}
}

//===============================end of pl===============================

// IsRunning checks to see if the OMXPlayer process is running. If it is, the
// function returns true, otherwise it returns false.
func (p *Player) IsRunning() bool {
	return p.condStart.Get()
}

func (p *Player) WaitFinishCurrentPlaying() {
	if p.condStart.Get() {
		p.condFinishCurrentPlaying.WaitSignal()
	}
}

// IsReady checks to see if the Player instance is ready to accept D-Bus
// commands. If the player is ready and can accept commands, the function
// returns true, otherwise it returns false.
func (p *Player) IsReady() bool {
	result, err := p.CmdCanQuit()
	if err == nil && result {
		return true
	} else {
		return false
	}
}

// WaitForReady waits until the Player instance is ready to accept D-Bus
// commands and then returns.
func (p *Player) WaitForReady() {
	for !p.IsReady() {
		time.Sleep(50 * time.Millisecond)
	}
}

func (p *Player) WaitForReadyWithTimeOut(timeout time.Duration) bool {
	timeoutAt := time.Now().Add(timeout)
	for !p.IsReady() {
		if time.Now().After(timeoutAt) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
	return true
}

func (p *Player) WaitForQuitTimeOut(timeout time.Duration) bool {
	timeoutAt := time.Now().Add(timeout)
	for p.IsReady() {
		if time.Now().Unix() > timeoutAt.Unix() {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
	return true
}

// Quit stops the currently playing video and terminates the omxplayer process.
// See https://github.com/popcornmix/omxplayer#quit for more details.
func (p *Player) CmdQuit() error {
	return sutils.DbusCall(p.bus, cmdQuit)
}

// CanQuit returns true if the player can quit, false otherwise. See
// https://github.com/popcornmix/omxplayer#canquit for more details.
func (p *Player) CmdCanQuit() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanQuit)
}

// Fullscreen returns true if the player is fullscreen, false otherwise. See
// https://github.com/popcornmix/omxplayer#fullscreen for more details.
func (p *Player) CmdFullscreen() (bool, error) {
	return sutils.DbusGetBool(p.bus, propFullscreen)
}

// CanSetFullscreen returns true if the player can be set to fullscreen, false
// otherwise. See https://github.com/popcornmix/omxplayer#cansetfullscreen for
// more details.
func (p *Player) CmdCanSetFullscreen() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanSetFullscreen)
}

// CanRaise returns true if the player can be brought to the front, false
// otherwise. See https://github.com/popcornmix/omxplayer#canraise for more
// details.
func (p *Player) CmdCanRaise() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanRaise)
}

// HasTrackList returns true if the player has a track list, false otherwise.
// See https://github.com/popcornmix/omxplayer#hastracklist for more details.
func (p *Player) CmdHasTrackList() (bool, error) {
	return sutils.DbusGetBool(p.bus, propHasTrackList)
}

// Identity returns the name of the player instance. See
// https://github.com/popcornmix/omxplayer#identity for more details.
func (p *Player) CmdIdentity() (string, error) {
	return sutils.DbusGetString(p.bus, propIdentity)
}

// SupportedURISchemes returns a list of playable URI formats. See
// https://github.com/popcornmix/omxplayer#supportedurischemes for more details.
func (p *Player) CmdSupportedURISchemes() ([]string, error) {
	return sutils.DbusGetStringArray(p.bus, propSupportedURISchemes)
}

// SupportedMimeTypes returns a list of supported MIME types. See
// https://github.com/popcornmix/omxplayer#supportedmimetypes for more details.
func (p *Player) CmdSupportedMimeTypes() ([]string, error) {
	return sutils.DbusGetStringArray(p.bus, propSupportedMimeTypes)
}

// CanGoNext returns true if the player can skip to the next track, false
// otherwise. See https://github.com/popcornmix/omxplayer#cangonext for more
// details.
func (p *Player) CmdCanGoNext() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanGoNext)
}

// CanGoPrevious returns true if the player can skip to previous track, false
// otherwise. See https://github.com/popcornmix/omxplayer#cangoprevious for more
// details.
func (p *Player) CmdCanGoPrevious() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanGoPrevious)
}

// CanSeek returns true if the player can seek, false otherwise. See
// https://github.com/popcornmix/omxplayer#canseek for more details.
func (p *Player) CmdCanSeek() (bool, error) {
	return sutils.DbusGetBool(p.bus, cmdSeek)
}

// CanControl returns true if the player can be controlled, false otherwise. See
// https://github.com/popcornmix/omxplayer#cancontrol for more details.
func (p *Player) CmdCanControl() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanControl)
}

// CanPlay returns true if the player can play, false otherwise. See
// https://github.com/popcornmix/omxplayer#canplay for more details.
func (p *Player) CmdCanPlay() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanPlay)
}

// CanPause returns true if the player can pause, false otherwise. See
// https://github.com/popcornmix/omxplayer#canpause for more details.
func (p *Player) CmdCanPause() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanPause)
}

// Next tells the player to skip to the next chapter. See
// https://github.com/popcornmix/omxplayer#next for more details.
func (p *Player) CmdNextTrack() error {
	return sutils.DbusCall(p.bus, cmdNext)
}

// Previous tells the player to skip to the previous chapter. See
// https://github.com/popcornmix/omxplayer#previous for more details.
func (p *Player) CmdPreviousTrack() error {
	return sutils.DbusCall(p.bus, cmdPrevious)
}

// Pause pauses the player if it is playing. Otherwise, it resumes playback. See
// https://github.com/popcornmix/omxplayer#pause for more details.
func (p *Player) CmdPause() error {
	p.command.Stdin = strings.NewReader(keyPause)
	return sutils.DbusCall(p.bus, cmdPause)
}

// Play play the video. If the video is playing, it has no effect,
// if it is paused it will play from current position.
// See https://github.com/popcornmix/omxplayer#play for more details.
func (p *Player) CmdPlay() error {
	return sutils.DbusCall(p.bus, cmdPlay)
}

// PlayPause pauses the player if it is playing. Otherwise, it resumes playback.
// See https://github.com/popcornmix/omxplayer#playpause for more details.
func (p *Player) CmdPlayPause() error {
	return sutils.DbusCall(p.bus, cmdPlayPause)
}

// Stop tells the player to stop playing the video. See
// https://github.com/popcornmix/omxplayer#stop for more details.
func (p *Player) CmdStop() bool {
	if p.IsRunning() || p.IsReady() {
		sutils.DbusCall(p.bus, cmdStop)
		if p.command.ProcessState == nil && p.command.Process != nil {
			syscall.Kill(-p.command.Process.Pid, syscall.SIGKILL)
			p.command.Process.Kill()
		}
		p.command.Wait()
		return true
		// return !p.IsRunning()
	}
	return true
}

// Seek performs a relative seek from the current video position. See
// https://github.com/popcornmix/omxplayer#seek for more details.
func (p *Player) CmdSeek(amount int64) (int64, error) {
	//	log.WithFields(log.Fields{
	//		"path":        cmdSeek,
	//		"paramAmount": amount,
	//	}).Debug("omxplayer: dbus call")
	call := p.bus.Call(cmdSeek, 0, amount)
	if call.Err != nil {
		return 0, call.Err
	}
	return call.Body[0].(int64), nil
}

// SetPosition performs an absolute seek to the specified video position. See
// https://github.com/popcornmix/omxplayer#setposition for more details.
func (p *Player) CmdSetPosition(path string, position int64) (int64, error) {
	//	log.WithFields(log.Fields{
	//		"path":          cmdSetPosition,
	//		"paramPath":     path,
	//		"paramPosition": position,
	//	}).Debug("omxplayer: dbus call")
	call := p.bus.Call(cmdSetPosition, 0, dbus.ObjectPath(path), position)
	if call.Err != nil {
		return 0, call.Err
	}
	return call.Body[0].(int64), nil
}

// PlaybackStatus returns the current state of the player. See
// https://github.com/popcornmix/omxplayer#playbackstatus for more details.
//The current state of the player, either "Paused" or "Playing".

func (p *Player) CmdPlaybackStatus() (string, error) {
	return sutils.DbusGetString(p.bus, propPlaybackStatus)
}

func (p *Player) CmdGetSource() (string, error) {
	return sutils.DbusGetString(p.bus, cmdGetSource)
}

func (p *Player) CmdOpenUri(uripath string) error {
	call := p.bus.Call(cmdOpenUri, 0, uripath)
	if call.Err != nil {
		return call.Err
	}
	return nil
}

func (p *Player) CmdRaise() (bool, error) {
	return sutils.DbusGetBool(p.bus, cmdRaise)
}

// Volume returns the current volume. Sets a new volume when an argument is
// specified. See https://github.com/popcornmix/omxplayer#volume for more
// details.
func (p *Player) CmdVolume(volume ...float64) (float64, error) {
	//	log.WithFields(log.Fields{
	//		"path":        cmdVolume,
	//		"paramVolume": volume,
	//	}).Debug("omxplayer: dbus call")
	if len(volume) == 0 {
		return sutils.DbusGetFloat64(p.bus, cmdVolume)
	}
	call := p.bus.Call(cmdVolume, 0, volume[0])
	if call.Err != nil {
		return 0, call.Err
	}
	p.currentVolume = call.Body[0].(float64)
	return p.currentVolume, nil
}

// Volume returns the current volume. Sets a new volume when an argument is
// specified. See https://github.com/popcornmix/omxplayer#volume for more
// details.
func (p *Player) CmdVolumePercent(volume ...int) (volint int, err error) {
	var volfloat float64
	volfloat = 0
	if len(volume) != 0 {
		vol := float64(volume[0]) / 100
		volfloat, err = p.CmdVolume(vol)
	} else {
		volfloat, err = p.CmdVolume()
	}
	volint = int(volfloat * 100)
	return volint, err
}

// Mute mutes the video's audio stream. See
// https://github.com/popcornmix/omxplayer#mute for more details.
func (p *Player) CmdMute() error {
	return sutils.DbusCall(p.bus, cmdMute)
}

// Unmute unmutes the video's audio stream. See
// https://github.com/popcornmix/omxplayer#unmute for more details.
func (p *Player) CmdUnmute() error {
	return sutils.DbusCall(p.bus, cmdUnmute)
}

// Position returns the current position in the video in milliseconds. See
// https://github.com/popcornmix/omxplayer#position for more details.
func (p *Player) Position() (int64, error) {
	return sutils.DbusGetInt64(p.bus, propPosition)
}

// Aspect returns the aspect ratio. See
// https://github.com/popcornmix/omxplayer/blob/master/OMXControl.cpp#L362.
func (p *Player) CmdAspect() (float64, error) {
	return sutils.DbusGetFloat64(p.bus, propAspect)
}

// VideoStreamCount returns the number of available video streams. See
// https://github.com/popcornmix/omxplayer/blob/master/OMXControl.cpp#L369.
func (p *Player) CmdVideoStreamCount() (int64, error) {
	return sutils.DbusGetInt64(p.bus, propVideoStreamCount)
}

// ResWidth returns the width of the video. See
// https://github.com/popcornmix/omxplayer/blob/master/OMXControl.cpp#L376.
func (p *Player) CmdResWidth() (int64, error) {
	return sutils.DbusGetInt64(p.bus, propResWidth)
}

// ResHeight returns the height of the video. See
// https://github.com/popcornmix/omxplayer/blob/master/OMXControl.cpp#L383.
func (p *Player) ResHeight() (int64, error) {
	return sutils.DbusGetInt64(p.bus, propResHeight)
}

// Duration returns the total length of the video in milliseconds. See
// https://github.com/popcornmix/omxplayer#duration for more details.
func (p *Player) CmdDuration() (int64, error) {
	return sutils.DbusGetInt64(p.bus, propDuration)
}

// MinimumRate returns the minimum playback rate. See
// https://github.com/popcornmix/omxplayer#minimumrate for more details.
func (p *Player) CmdMinimumRate() (float64, error) {
	return sutils.DbusGetFloat64(p.bus, propMinimumRate)
}

// MaximumRate returns the maximum playback rate. See
// https://github.com/popcornmix/omxplayer#maximumrate for more details.
func (p *Player) CmdMaximumRate() (float64, error) {
	return sutils.DbusGetFloat64(p.bus, propMaximumRate)
}

// ListSubtitles returns a list of the subtitles available in the video file.
// See https://github.com/popcornmix/omxplayer#listsubtitles for more details.
func (p *Player) ListSubtitles() ([]string, error) {
	return sutils.DbusGetStringArray(p.bus, cmdListSubtitles)
}

// HideVideo is an undocumented D-Bus method. See
// https://github.com/popcornmix/omxplayer/blob/master/OMXControl.cpp#L457.
func (p *Player) CmdHideVideo() error {
	return sutils.DbusCall(p.bus, cmdHideVideo)
}

// UnHideVideo is an undocumented D-Bus method. See
// https://github.com/popcornmix/omxplayer/blob/master/OMXControl.cpp#L462.
func (p *Player) CmdUnHideVideo() error {
	return sutils.DbusCall(p.bus, cmdUnHideVideo)
}

// ListAudio returns a list of the audio tracks available in the video file. See
// https://github.com/popcornmix/omxplayer#listaudio for more details.
func (p *Player) ListAudio() ([]string, error) {
	return sutils.DbusGetStringArray(p.bus, cmdListAudio)
}

// ListVideo returns a list of the video tracks available in the video file. See
// https://github.com/popcornmix/omxplayer#listvideo for more details.
func (p *Player) CmdListVideo() ([]string, error) {
	return sutils.DbusGetStringArray(p.bus, cmdListVideo)
}

// SelectSubtitle specifies which subtitle track should be used. See
// https://github.com/popcornmix/omxplayer#selectsubtitle for more details.
func (p *Player) CmdSelectSubtitle(index int32) (bool, error) {
	//	log.WithFields(log.Fields{
	//		"path":       cmdSelectSubtitle,
	//		"paramIndex": index,
	//	}).Debug("omxplayer: dbus call")
	call := p.bus.Call(cmdSelectSubtitle, 0, index)
	if call.Err != nil {
		return false, call.Err
	}
	return call.Body[0].(bool), nil
}

// SelectAudio specifies which audio track should be used. See
// https://github.com/popcornmix/omxplayer#selectaudio for more details.
func (p *Player) CmdSelectAudio(index int32) (bool, error) {
	//	log.WithFields(log.Fields{
	//		"path":       cmdSelectAudio,
	//		"paramIndex": index,
	//	}).Debug("omxplayer: dbus call")
	call := p.bus.Call(cmdSelectAudio, 0, index)
	if call.Err != nil {
		return false, call.Err
	}
	return call.Body[0].(bool), nil
}

// ShowSubtitles starts displaying subtitles. See
// https://github.com/popcornmix/omxplayer#showsubtitles for more details.
func (p *Player) CmdShowSubtitles() error {
	return sutils.DbusCall(p.bus, cmdShowSubtitles)
}

// HideSubtitles stops displaying subtitles. See
// https://github.com/popcornmix/omxplayer#hidesubtitles for more details.
func (p *Player) CmdHideSubtitles() error {
	return sutils.DbusCall(p.bus, cmdHideSubtitles)
}

// Action allows for executing keyboard commands. See
// https://github.com/popcornmix/omxplayer#action for more details.
func (p *Player) CmdAction(action int32) error {
	//	log.WithFields(log.Fields{
	//		"path":        cmdAction,
	//		"paramAction": action,
	//	}).Debug("omxplayer: dbus call")
	return p.bus.Call(cmdAction, 0, action).Err
}

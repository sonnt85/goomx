// +build linux,arm
package goomx

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	dbus "github.com/godbus/dbus"

	//	dbus "github.com/guelfey/go.dbus"
	"github.com/sonnt85/gosutils/sexec"
	"github.com/sonnt85/gosutils/sutils"

	log "github.com/sirupsen/logrus"
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
type Player struct {
	command        *exec.Cmd
	connection     *dbus.Conn
	bus            *dbus.Object
	ready          bool
	argsOmx        []string
	currentVolume  float64
	activePlaylist bool
	enablePlay     bool
	indexRunning   int
	playlist       []string
	videodir       string
	mutex          *sync.Mutex
	once           sync.Once
	cstop          chan struct{}
	cplay          chan struct{}
	cpause         chan struct{}
}

func (p *Player) PlNextVideo(updateIndex bool) (retfile string) {
	//	p.mutex.Lock()
	//	defer p.mutex.Unlock()
	if len(p.playlist) != 0 {
		tmpindex := p.indexRunning + 1
		if tmpindex >= len(p.playlist) {
			tmpindex = 0
		}
		if updateIndex {
			p.indexRunning = tmpindex
		}
		//		fmt.Println(p.enablePlay, len(p.playlist), tmpindex, p.playlist[tmpindex])
		return p.playlist[tmpindex]
	}
	return
}

func (p *Player) PlPrevVideo(updateIndex bool) (retfile string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if len(p.playlist) != 0 {
		tmpindex := p.indexRunning - 1
		if tmpindex < 0 {
			tmpindex = len(p.playlist) - 1
		}
		if updateIndex {
			p.indexRunning = tmpindex
		}
		return p.playlist[tmpindex]
	}
	return
}

func (p *Player) PlGetRunningVideo() (retfile string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if len(p.playlist) != 0 {
		return p.playlist[p.indexRunning]
	}
	return
}

func (p *Player) PlVideoIsExists(f string) (retbool bool) {
	retbool = sutils.PathIsFile(p.videodir + f)
	return retbool
}

func (p *Player) PlDeleteVideos(files []string) ([]string, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	errlist := []string{}
	for _, f := range files {
		fullPath := p.videodir + f
		if sutils.PathIsFile(fullPath) {
			if os.Remove(fullPath) != nil {
				errlist = append(errlist, f)
			}
		} else {
			errlist = append(errlist, f)
		}
	}
	if len(errlist) != 0 {
		return errlist, errors.New("Some files can not deleted")
	} else {
		return errlist, nil
	}
}

func (p *Player) PlAddVideoToPlaylist(finename string, index int) bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	tmpplaylist := make([]string, 0)

	if p.PlVideoIsExists(finename) {
		for i := 0; i < len(p.playlist); i++ {
			if i != index {
				tmpplaylist = append(tmpplaylist, p.playlist[i])
			} else {
				tmpplaylist = append(tmpplaylist, finename)
			}
		}
		p.playlist = tmpplaylist
		return true
	} else {
		return false
	}
}

func (p *Player) PlDeleteVideoFromPlaylist(index int) bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	tmpplaylist := make([]string, 0)
	for i := 0; i < len(p.playlist); i++ {
		if i != index {
			tmpplaylist = append(tmpplaylist, p.playlist[i])
		} else {
			if len(p.playlist) == (p.indexRunning - 1) {
				if len(p.playlist) >= 2 {
					p.indexRunning = len(p.playlist) - 2
				} else {
					p.indexRunning = 0
				}
			}
		}
	}
	p.playlist = tmpplaylist
	return true
}

func (p *Player) PlGetVideosRoot() string {
	return p.videodir
}

func (p *Player) PlSetVideoRoot(pathdir string) {
	p.videodir = pathdir
}

func (p *Player) PlGetListVideos() []string {
	retstrs := make([]string, 0)
	for _, v := range sutils.FindFile(p.videodir) {
		retstrs = append(retstrs, filepath.Base(v))
	}
	return retstrs
}

func (p *Player) PlGetPlaylist() []string {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	retstrs := make([]string, 0)
	for i := 0; i < len(p.playlist); i++ {
		filename := filepath.Base(p.playlist[i])
		if i == p.indexRunning {
			filename = "[[Playing]]" + filename
		}
		retstrs = append(retstrs, filename)
	}
	return retstrs
}

func (p *Player) PlViewDefaultPictures(picspath string) {
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
			for {
				if sutils.PathIsDir(picspath) {
					sexec.ExecCommandShell("omxiv  -t 3 -a center --transition blend --duration 3000 "+picspath, 0)
				} else {
					sexec.ExecCommandShell("omxiv  -a center --transition blend --duration 3000 "+picspath, 0)
				}

				time.Sleep(time.Second * 1)
			}
		}()
	}

}

func (p *Player) PlRunPlaylist() {
	var err error
	p.PlConfigurePlaylist([]string{}) //load all video
	fmt.Println("Staring run playlist", p.PlGetVideosRoot()+":", p.PlGetPlaylist(), "...")
	defer func() {
		fmt.Println("Exit playlist mode")
		p.once = sync.Once{}
	}()
	waitingEndVideo := false
	videofile := ""

	for {
		//	err = player.ShowSubtitles()
	loopcheck:
		for {
			//			if time.Now().After(stopAt) {
			//				p.Stop()
			//				break
			//			}

			//			if _, err := p.PlaybackStatus(); err != nil {
			//				break
			//			}
			ticker := time.After(time.Millisecond * 25)
			select {
			case <-p.cstop:
				fmt.Println("Time Out!")
			case <-ticker:
				if !p.PlIsPlaylistModeEnabled() {
					fmt.Println("Playlist mode is disabled!")
					p.Stop()
					return
				}

				if !p.PlIsEnablePlay() {
					//			fmt.Println("Play is disabled!")
					p.Stop()
					continue
				}

				if !waitingEndVideo {
					break loopcheck
				} else {
					if !p.IsReady() {
						fmt.Println("Done play video", videofile)
						break loopcheck
					}
				}
			}
		}
		waitingEndVideo = false
		p.PlAutoCleanup()

		videofile = p.PlNextVideo(true)
		if len(videofile) == 0 {
			videofile = "/demo.mp4"
			if !sutils.PathIsFile(videofile) {
				fmt.Println("videofile len is 0", videofile)
				continue
			}
		}

		fmt.Println("Loading new video", videofile)
		start := time.Now()
		timeoutLoadVideo := time.Now().Add(time.Second * 10)
		errPlay := true

		for {
			if time.Now().After(timeoutLoadVideo) {
				break
			}
			if err = p.PlLoadVideo(videofile); err != nil {
				fmt.Println("Can not load video", videofile, err)
				continue
			}
			//player
			//		dbuspid, _ := getDbusPid()
			//		dbusadd, _ := getDbusPath()
			//		fmt.Println("Waiting dbus", dbuspid, dbusadd)

			if !p.WaitForReadyWithTimeOut(time.Millisecond * 3000) {
				fmt.Println("Timeout for wating omxplayer start after", time.Since(start))
				continue
			}
			errPlay = false
			break
		}
		if errPlay {
			fmt.Println("Total Timeout for wating omxplayer start after", time.Since(start))
			continue
		}
		fmt.Println("Time to load video: ", time.Since(start))
		//		fmt.Println("Dbus is running")
		p.Raise()
		if _, err = p.Volume(p.PlGetSavedVolume()); err != nil {
			fmt.Println("Can not Set Volume", err)
			continue
		}

		if err = p.Play(); err != nil {
			fmt.Println("Error Play()", videofile, err)
			continue
		}

		durmilis, _ := p.Duration()

		//		stopAt := time.Now().Add(time.Duration(durmilis)*time.Millisecond - 1000)
		//		p.Fullscreen()
		//		time.Sleep(time.Second * 2)
		//		if mitype, err := p.SupportedMimeTypes(); err == nil {
		//			fmt.Println("SupportedMimeTypes", mitype)
		//		}
		fmt.Println("Playing video", videofile, durmilis/1000000, "seconds (", p.command.Process.Pid, p.IsRunning(), `)`)
		//		fmt.Println("Pid status", p.command.ProcessState)
		waitingEndVideo = true
	}
}

func (p *Player) PlEnablePlaylistMode() {
	p.activePlaylist = true
	go p.once.Do(p.PlRunPlaylist)
}

func (p *Player) PlDisablePlaylistMode() {
	p.activePlaylist = false
}

func (p *Player) PlIsPlaylistModeEnabled() bool {
	return p.activePlaylist
}

func (p *Player) PlDisablePlay() {
	if srcpath, err := p.GetSource(); err == nil && srcpath == "/demo.mp4" {
		return
	}
	p.Stop()
	p.enablePlay = false
}

func (p *Player) PlEnablePlay() {
	p.enablePlay = true
}

func (p *Player) PlIsEnablePlay() bool {
	return p.enablePlay
}

func (p *Player) PlGetSavedVolume() float64 {
	return p.currentVolume
}

func (p *Player) PlConfigurePlaylist(list []string) []string {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	oldlist := p.playlist
	if len(list) == 0 {
		p.playlist = sutils.FindFile(p.videodir)
	} else {
		for i := 0; i < len(list); i++ {
			list[i] = p.videodir + list[i]
		}
		p.playlist = list
	}
	if !reflect.DeepEqual(oldlist, p.playlist) {
		fmt.Println("PlConfigurePlaylist", oldlist, p.playlist)
		if p.command != nil {
			p.Stop()
		}
		p.indexRunning = 0
	}
	//	sort.Strings(p.playlist)
	return p.playlist
	//	fmt.Println("videolist", p.playlist)
}

//cleanup omxplayer
func (p *Player) PlAutoCleanup() bool {
	if p.command != nil && !p.IsReady() && p.IsRunning() {
		p.command.Process.Kill()
		p.command.Wait()
		return false
	} else {
		return true
	}
}
func (p *Player) PlLoadVideo(url string, args ...string) (err error) {
	//	fmt.Println("omxplayer: Loading new video Lock")

	p.mutex.Lock()
	defer p.mutex.Unlock()
	//	fmt.Println("omxplayer: Loading new video")
	if p.command != nil {
		p.Stop()
	}
	//	removeDbusFiles()

	if len(args) == 0 {
		args = append(p.argsOmx, url)
	} else {
		args = append(args, url)
	}
	cmd := exec.Command(exeOxmPlayer, args...)
	//	cmd.CombinedOutput()
	cmd.Stdin = strings.NewReader(keyPause)
	err = cmd.Start()
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

	p.command = cmd
	p.connection = conn
	p.bus = conn.Object(ifaceOmx, pathMpris).(*dbus.Object)
	return
}

//===============================end of pl===============================

// IsRunning checks to see if the OMXPlayer process is running. If it is, the
// function returns true, otherwise it returns false.
func (p *Player) IsRunning() bool {
	if p.command == nil {
		return false
	}
	return sutils.IsProcessAlive(p.command.Process.Pid)
}

// IsReady checks to see if the Player instance is ready to accept D-Bus
// commands. If the player is ready and can accept commands, the function
// returns true, otherwise it returns false.
func (p *Player) IsReady() bool {
	result, err := p.CanQuit()
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
func (p *Player) Quit() error {
	//	return p.command.Process.Kill()
	return sutils.DbusCall(p.bus, cmdQuit)
}

// CanQuit returns true if the player can quit, false otherwise. See
// https://github.com/popcornmix/omxplayer#canquit for more details.
func (p *Player) CanQuit() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanQuit)
}

// Fullscreen returns true if the player is fullscreen, false otherwise. See
// https://github.com/popcornmix/omxplayer#fullscreen for more details.
func (p *Player) Fullscreen() (bool, error) {
	return sutils.DbusGetBool(p.bus, propFullscreen)
}

// CanSetFullscreen returns true if the player can be set to fullscreen, false
// otherwise. See https://github.com/popcornmix/omxplayer#cansetfullscreen for
// more details.
func (p *Player) CanSetFullscreen() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanSetFullscreen)
}

// CanRaise returns true if the player can be brought to the front, false
// otherwise. See https://github.com/popcornmix/omxplayer#canraise for more
// details.
func (p *Player) CanRaise() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanRaise)
}

// HasTrackList returns true if the player has a track list, false otherwise.
// See https://github.com/popcornmix/omxplayer#hastracklist for more details.
func (p *Player) HasTrackList() (bool, error) {
	return sutils.DbusGetBool(p.bus, propHasTrackList)
}

// Identity returns the name of the player instance. See
// https://github.com/popcornmix/omxplayer#identity for more details.
func (p *Player) Identity() (string, error) {
	return sutils.DbusGetString(p.bus, propIdentity)
}

// SupportedURISchemes returns a list of playable URI formats. See
// https://github.com/popcornmix/omxplayer#supportedurischemes for more details.
func (p *Player) SupportedURISchemes() ([]string, error) {
	return sutils.DbusGetStringArray(p.bus, propSupportedURISchemes)
}

// SupportedMimeTypes returns a list of supported MIME types. See
// https://github.com/popcornmix/omxplayer#supportedmimetypes for more details.
func (p *Player) SupportedMimeTypes() ([]string, error) {
	return sutils.DbusGetStringArray(p.bus, propSupportedMimeTypes)
}

// CanGoNext returns true if the player can skip to the next track, false
// otherwise. See https://github.com/popcornmix/omxplayer#cangonext for more
// details.
func (p *Player) CanGoNext() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanGoNext)
}

// CanGoPrevious returns true if the player can skip to previous track, false
// otherwise. See https://github.com/popcornmix/omxplayer#cangoprevious for more
// details.
func (p *Player) CanGoPrevious() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanGoPrevious)
}

// CanSeek returns true if the player can seek, false otherwise. See
// https://github.com/popcornmix/omxplayer#canseek for more details.
func (p *Player) CanSeek() (bool, error) {
	return sutils.DbusGetBool(p.bus, cmdSeek)
}

// CanControl returns true if the player can be controlled, false otherwise. See
// https://github.com/popcornmix/omxplayer#cancontrol for more details.
func (p *Player) CanControl() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanControl)
}

// CanPlay returns true if the player can play, false otherwise. See
// https://github.com/popcornmix/omxplayer#canplay for more details.
func (p *Player) CanPlay() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanPlay)
}

// CanPause returns true if the player can pause, false otherwise. See
// https://github.com/popcornmix/omxplayer#canpause for more details.
func (p *Player) CanPause() (bool, error) {
	return sutils.DbusGetBool(p.bus, propCanPause)
}

// Next tells the player to skip to the next chapter. See
// https://github.com/popcornmix/omxplayer#next for more details.
func (p *Player) NextTrack() error {
	return sutils.DbusCall(p.bus, cmdNext)
}

// Previous tells the player to skip to the previous chapter. See
// https://github.com/popcornmix/omxplayer#previous for more details.
func (p *Player) PreviousTrack() error {
	return sutils.DbusCall(p.bus, cmdPrevious)
}

// Pause pauses the player if it is playing. Otherwise, it resumes playback. See
// https://github.com/popcornmix/omxplayer#pause for more details.
func (p *Player) Pause() error {
	p.command.Stdin = strings.NewReader(keyPause)
	return sutils.DbusCall(p.bus, cmdPause)
}

// Play play the video. If the video is playing, it has no effect,
// if it is paused it will play from current position.
// See https://github.com/popcornmix/omxplayer#play for more details.
func (p *Player) Play() error {
	return sutils.DbusCall(p.bus, cmdPlay)
}

// PlayPause pauses the player if it is playing. Otherwise, it resumes playback.
// See https://github.com/popcornmix/omxplayer#playpause for more details.
func (p *Player) PlayPause() error {
	return sutils.DbusCall(p.bus, cmdPlayPause)
}

// Stop tells the player to stop playing the video. See
// https://github.com/popcornmix/omxplayer#stop for more details.
func (p *Player) Stop() (err error) {
	//	p.command.Stdin = strings.NewReader(keyQuit)

	if p.IsRunning() || p.IsReady() {
		start := time.Now()
		err = sutils.DbusCall(p.bus, cmdStop)
		//		p.command.Stdin = strings.NewReader(keyQuit)
		//		fmt.Println("cmdStop done")
		p.command.Process.Kill()
		p.command.Wait()
		fmt.Println("Timeout for quit omxplayerr", time.Since(start))

		//		fmt.Println("Wait done")
		//		fmt.Println("Status omx", p.command.ProcessState)
		//		fmt.Println("p.command.Process.Kill()", p.command.Process.Kill())
		//		fmt.Println("release omx resource", p.command.Process.Release())
	}
	if _, _, err := sexec.ExecCommandShell(`pkill -9 `+exeOxmPlayer, time.Second*1); err != nil {
		//		checkLTE("/dev/ttyUSB2")
	}
	return err
}

// Seek performs a relative seek from the current video position. See
// https://github.com/popcornmix/omxplayer#seek for more details.
func (p *Player) Seek(amount int64) (int64, error) {
	log.WithFields(log.Fields{
		"path":        cmdSeek,
		"paramAmount": amount,
	}).Debug("omxplayer: dbus call")
	call := p.bus.Call(cmdSeek, 0, amount)
	if call.Err != nil {
		return 0, call.Err
	}
	return call.Body[0].(int64), nil
}

// SetPosition performs an absolute seek to the specified video position. See
// https://github.com/popcornmix/omxplayer#setposition for more details.
func (p *Player) SetPosition(path string, position int64) (int64, error) {
	log.WithFields(log.Fields{
		"path":          cmdSetPosition,
		"paramPath":     path,
		"paramPosition": position,
	}).Debug("omxplayer: dbus call")
	call := p.bus.Call(cmdSetPosition, 0, dbus.ObjectPath(path), position)
	if call.Err != nil {
		return 0, call.Err
	}
	return call.Body[0].(int64), nil
}

// PlaybackStatus returns the current state of the player. See
// https://github.com/popcornmix/omxplayer#playbackstatus for more details.
//The current state of the player, either "Paused" or "Playing".

func (p *Player) PlaybackStatus() (string, error) {
	return sutils.DbusGetString(p.bus, propPlaybackStatus)
}

func (p *Player) GetSource() (string, error) {
	return sutils.DbusGetString(p.bus, cmdGetSource)
}

func (p *Player) cmdOpenUri(uripath string) error {
	call := p.bus.Call(cmdOpenUri, 0, uripath)
	if call.Err != nil {
		return call.Err
	}
	return nil
}

func (p *Player) Raise() (bool, error) {
	return sutils.DbusGetBool(p.bus, cmdRaise)
}

// Volume returns the current volume. Sets a new volume when an argument is
// specified. See https://github.com/popcornmix/omxplayer#volume for more
// details.
func (p *Player) Volume(volume ...float64) (float64, error) {
	log.WithFields(log.Fields{
		"path":        cmdVolume,
		"paramVolume": volume,
	}).Debug("omxplayer: dbus call")
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
func (p *Player) VolumePercent(volume ...int) (volint int, err error) {
	var volfloat float64
	volfloat = 0
	if len(volume) != 0 {
		vol := float64(volume[0]) / 100
		volfloat, err = p.Volume(vol)
	} else {
		volfloat, err = p.Volume()
	}
	volint = int(volfloat * 100)
	return volint, err
}

// Mute mutes the video's audio stream. See
// https://github.com/popcornmix/omxplayer#mute for more details.
func (p *Player) Mute() error {
	return sutils.DbusCall(p.bus, cmdMute)
}

// Unmute unmutes the video's audio stream. See
// https://github.com/popcornmix/omxplayer#unmute for more details.
func (p *Player) Unmute() error {
	return sutils.DbusCall(p.bus, cmdUnmute)
}

// Position returns the current position in the video in milliseconds. See
// https://github.com/popcornmix/omxplayer#position for more details.
func (p *Player) Position() (int64, error) {
	return sutils.DbusGetInt64(p.bus, propPosition)
}

// Aspect returns the aspect ratio. See
// https://github.com/popcornmix/omxplayer/blob/master/OMXControl.cpp#L362.
func (p *Player) Aspect() (float64, error) {
	return sutils.DbusGetFloat64(p.bus, propAspect)
}

// VideoStreamCount returns the number of available video streams. See
// https://github.com/popcornmix/omxplayer/blob/master/OMXControl.cpp#L369.
func (p *Player) VideoStreamCount() (int64, error) {
	return sutils.DbusGetInt64(p.bus, propVideoStreamCount)
}

// ResWidth returns the width of the video. See
// https://github.com/popcornmix/omxplayer/blob/master/OMXControl.cpp#L376.
func (p *Player) ResWidth() (int64, error) {
	return sutils.DbusGetInt64(p.bus, propResWidth)
}

// ResHeight returns the height of the video. See
// https://github.com/popcornmix/omxplayer/blob/master/OMXControl.cpp#L383.
func (p *Player) ResHeight() (int64, error) {
	return sutils.DbusGetInt64(p.bus, propResHeight)
}

// Duration returns the total length of the video in milliseconds. See
// https://github.com/popcornmix/omxplayer#duration for more details.
func (p *Player) Duration() (int64, error) {
	return sutils.DbusGetInt64(p.bus, propDuration)
}

// MinimumRate returns the minimum playback rate. See
// https://github.com/popcornmix/omxplayer#minimumrate for more details.
func (p *Player) MinimumRate() (float64, error) {
	return sutils.DbusGetFloat64(p.bus, propMinimumRate)
}

// MaximumRate returns the maximum playback rate. See
// https://github.com/popcornmix/omxplayer#maximumrate for more details.
func (p *Player) MaximumRate() (float64, error) {
	return sutils.DbusGetFloat64(p.bus, propMaximumRate)
}

// ListSubtitles returns a list of the subtitles available in the video file.
// See https://github.com/popcornmix/omxplayer#listsubtitles for more details.
func (p *Player) ListSubtitles() ([]string, error) {
	return sutils.DbusGetStringArray(p.bus, cmdListSubtitles)
}

// HideVideo is an undocumented D-Bus method. See
// https://github.com/popcornmix/omxplayer/blob/master/OMXControl.cpp#L457.
func (p *Player) HideVideo() error {
	return sutils.DbusCall(p.bus, cmdHideVideo)
}

// UnHideVideo is an undocumented D-Bus method. See
// https://github.com/popcornmix/omxplayer/blob/master/OMXControl.cpp#L462.
func (p *Player) UnHideVideo() error {
	return sutils.DbusCall(p.bus, cmdUnHideVideo)
}

// ListAudio returns a list of the audio tracks available in the video file. See
// https://github.com/popcornmix/omxplayer#listaudio for more details.
func (p *Player) ListAudio() ([]string, error) {
	return sutils.DbusGetStringArray(p.bus, cmdListAudio)
}

// ListVideo returns a list of the video tracks available in the video file. See
// https://github.com/popcornmix/omxplayer#listvideo for more details.
func (p *Player) ListVideo() ([]string, error) {
	return sutils.DbusGetStringArray(p.bus, cmdListVideo)
}

// SelectSubtitle specifies which subtitle track should be used. See
// https://github.com/popcornmix/omxplayer#selectsubtitle for more details.
func (p *Player) SelectSubtitle(index int32) (bool, error) {
	log.WithFields(log.Fields{
		"path":       cmdSelectSubtitle,
		"paramIndex": index,
	}).Debug("omxplayer: dbus call")
	call := p.bus.Call(cmdSelectSubtitle, 0, index)
	if call.Err != nil {
		return false, call.Err
	}
	return call.Body[0].(bool), nil
}

// SelectAudio specifies which audio track should be used. See
// https://github.com/popcornmix/omxplayer#selectaudio for more details.
func (p *Player) SelectAudio(index int32) (bool, error) {
	log.WithFields(log.Fields{
		"path":       cmdSelectAudio,
		"paramIndex": index,
	}).Debug("omxplayer: dbus call")
	call := p.bus.Call(cmdSelectAudio, 0, index)
	if call.Err != nil {
		return false, call.Err
	}
	return call.Body[0].(bool), nil
}

// ShowSubtitles starts displaying subtitles. See
// https://github.com/popcornmix/omxplayer#showsubtitles for more details.
func (p *Player) ShowSubtitles() error {
	return sutils.DbusCall(p.bus, cmdShowSubtitles)
}

// HideSubtitles stops displaying subtitles. See
// https://github.com/popcornmix/omxplayer#hidesubtitles for more details.
func (p *Player) HideSubtitles() error {
	return sutils.DbusCall(p.bus, cmdHideSubtitles)
}

// Action allows for executing keyboard commands. See
// https://github.com/popcornmix/omxplayer#action for more details.
func (p *Player) Action(action int32) error {
	log.WithFields(log.Fields{
		"path":        cmdAction,
		"paramAction": action,
	}).Debug("omxplayer: dbus call")
	return p.bus.Call(cmdAction, 0, action).Err
}

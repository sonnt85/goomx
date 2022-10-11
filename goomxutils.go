package goomx

import (
	"github.com/golang/freetype/truetype"
	log "github.com/sirupsen/logrus"
	"github.com/sonnt85/gofb"
	"golang.org/x/image/font/gofont/goregular"
)

func (p *Player) _ActiveViewDefaultPictures(picspath string) {
	go func() {
		var dev *gofb.Device
		var err error
		dev, err = gofb.NewDevie("/dev/fb0")

		if err != nil {
			log.Println("Can not open fb0", err)
			return
		}

		font, err := truetype.Parse(goregular.TTF)
		if err != nil {
			log.Println("Cannot parser font", err)
			return
		}

		face := truetype.NewFace(font, &truetype.Options{Size: 72})
		// center := image.Pt(dev.W/2, dev.H/2)
		TEXT_SHOW := picspath + "fdddddfd"
		dev.SetRGB(1, 1, 1) //white color backgound
		dev.Clear()
		log.Printf("Resolution: %dx%d\n", dev.Width(), dev.Width())
		dev.SetFontFace(face)

		for {
			p.condStartViewPicture.TestThenWaitSignalIfNotMatch(true)
			p.condStartViewPicture.Set(false)
			//Here is a simple example that clears the whole screen to a dark magenta:
			// magenta := image.NewUniform(color.RGBA{245, 40, 145, 255})
			// draw.Draw(fb, fb.Bounds(), magenta, image.ZP, draw.Src)

			// dev.Push()
			// dev.SetColor(gofb.GetRandomColorInRgb(255))
			// dev.DrawCircle(float64(center.X), float64(center.Y), float64(math.Min(float64(rect.Dy()), float64(rect.Dx())/3)))
			// dev.Fill()
			// dev.Pop()
			// rgbcolour = gofb.GetRandomColorInRgb(255)
			// dev.SetColor(rgbcolour)
			// dev.SetRGB(1, 1, 1)
			// dev.Clear()
			dev.SetRGB(0, 0, 0)
			dev.DrawStringAnchored(TEXT_SHOW, float64(dev.Context.Width()/2), float64(dev.Width()/2), 0.5, 0.5)
			dev.Fill()
			// dev.SetRGB(0, 1, 0)

			// lines := dev.WordWrap(TEXT_SHOW, float64(rect.Dx()-2*20))
			// dev.DrawString(TEXT_SHOW, float64(rect.Dx()/2)-float64(len(TEXT_SHOW))/2, float64(rect.Dy()/2))
			// y := rect.Min.Y + 20
			// for i, l := range lines {
			// 	dev.DrawString(slogrus.Sprintf("%2d: %s", i, l), 5, float64(y))
			// 	dev.Fill()
			// 	y += 20
			// }

			// draw.Draw(dev, dev.Bounds(), dev.Image, image.ZP, draw.Src)

			p.condStopViewPicture.TestThenWaitSignalIfNotMatch(true)
			p.condStopViewPicture.Set(false)
		}
	}()
}

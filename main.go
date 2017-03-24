package main

import (
	"bufio"
	"flag"
	"image"
	"image/png"
	"io/ioutil"
	"os"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	size = 28.0
	DPI  = 72.0
)

var (
	long, short string
	f           *truetype.Font

	longTextBB = image.Rectangle{
		Min: image.Point{X: 60, Y: 75},
		Max: image.Point{X: 350, Y: 120},
	}
	shortTextBB = image.Rectangle{
		Min: image.Point{X: 140, Y: 215},
		Max: image.Point{X: 220, Y: 250},
	}
)

func main() {

	flag.StringVar(&long, "long", "compiling my code", "long text for the excuse")
	flag.StringVar(&short, "short", "compiling", "short text for the excuse")
	flag.Parse()

	// image template
	imgf, err := os.Open("./resources/xkcd-excuse-template.png")
	if err != nil {
		println("Error opening image", err.Error())
		os.Exit(1)
	}
	defer imgf.Close()

	img, err := png.Decode(imgf)
	if err != nil {
		println("Error decoding image template", err.Error())
		os.Exit(1)
	}

	var ok bool
	if img, ok = img.(*image.NRGBA); !ok {
		println("Could not type cast the image", err)
		os.Exit(1)
	}

	// fonts
	fontBytes, err := ioutil.ReadFile("./resources/xkcd.ttf")
	if err != nil {
		println("Error loading ttf file", err)
		os.Exit(1)
	}
	f, err = freetype.ParseFont(fontBytes)
	if err != nil {
		println("Error parsing font file", err)
		os.Exit(1)
	}

	dst := image.NewRGBA(img.Bounds())
	draw.Copy(dst, image.ZP, img, img.Bounds(), draw.Src, nil)

	// the complete excuse
	if err := drawString(`"`+long+`"`, size, &longTextBB, dst); err != nil {
		println(err.Error())
		os.Exit(1)
	}

	// the short excuse
	if err := drawString(short, size-2.0, &shortTextBB, dst); err != nil {
		println(err.Error())
		os.Exit(1)
	}

	// save
	outFile, err := os.Create("out.png")
	if err != nil {
		println("Error createing out file", err)
		os.Exit(1)
	}
	defer outFile.Close()
	b := bufio.NewWriter(outFile)
	if err = png.Encode(outFile, dst); err != nil {
		println("Error encoding image file", err)
		os.Exit(1)
	}
	if err = b.Flush(); err != nil {
		println("Error flushing buffer", err)
		os.Exit(1)
	}

	println("Wrote image file OK")
}

// drawString will try to draw the string text with size in the bounding box defined by bb on the image dst
// if bb is not nil then it will be checked whether the string wouldn't fit the bounding box,
// the size will be recalculated until it fits the bounding box
func drawString(text string, size float64, bb *image.Rectangle, dst *image.RGBA) error {
	s, startX := fitString(text, size, bb)
	fg := image.Black
	c := freetype.NewContext()
	c.SetDPI(DPI)
	c.SetFont(f)
	c.SetFontSize(s)
	c.SetClip(dst.Bounds())
	c.SetSrc(fg)
	c.SetDst(dst)
	c.SetHinting(font.HintingNone)

	pt := freetype.Pt(bb.Min.X, bb.Min.Y+(int(c.PointToFixed(size)>>6)))
	pt.X = startX
	_, err := c.DrawString(text, pt)
	return err
}

func fitString(text string, size float64, bb *image.Rectangle) (float64, fixed.Int26_6) {
	var adv fixed.Int26_6
	for {

		opts := &truetype.Options{
			Size: size,
			DPI:  DPI,
		}
		fFace := truetype.NewFace(f, opts)

		adv = font.MeasureString(fFace, text)
		bbMinXAsFixed := fixed.I(bb.Min.X)
		bbMaxXAsFixed := fixed.I(bb.Max.X)

		if bbMinXAsFixed+adv < bbMaxXAsFixed {
			break
		}
		size -= 1.0
		//println("reduced")
	}

	bbWidth := bb.Max.X - bb.Min.X
	bbMiddle := bb.Min.X + bbWidth/2
	textStart := fixed.I(bbMiddle) - adv/2

	return size, textStart
}

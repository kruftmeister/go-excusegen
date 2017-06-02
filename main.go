package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"image"
	"image/png"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"sync"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/gorilla/mux"
	"github.com/mier85/goimgur"
	"github.com/pkg/errors"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

type (
	Uploader interface {
		Upload(string) (string, error)
	}
	Cacher interface {
		Set(Key, string)
		Get(Key) (string, bool)
	}

	Key struct {
		Short string
		Long  string
	}
	InMemoryCache struct {
		images map[Key]string
		mutex  *sync.RWMutex
	}
)

const (
	size = 28.0
	DPI  = 72.0
)

var (
	clientId = flag.String("clientID", "", "id for imgur client")
	port     = flag.Int("port", 18888, "port for server")

	f *truetype.Font

	longTextBB = image.Rectangle{
		Min: image.Point{X: 60, Y: 75},
		Max: image.Point{X: 350, Y: 120},
	}
	shortTextBB = image.Rectangle{
		Min: image.Point{X: 140, Y: 215},
		Max: image.Point{X: 220, Y: 250},
	}
)

func create(uploader Uploader, short, long string) (string, error) {
	// image template
	imgf, err := os.Open("./resources/xkcd-excuse-template.png")
	if err != nil {
		return "", err
	}
	defer imgf.Close()

	img, err := png.Decode(imgf)
	if err != nil {
		return "", err
	}

	var ok bool
	if img, ok = img.(*image.NRGBA); !ok {
		return "", err
	}

	// fonts
	fontBytes, err := ioutil.ReadFile("./resources/xkcd.ttf")
	if err != nil {
		return "", err
	}
	f, err = freetype.ParseFont(fontBytes)
	if err != nil {
		return "", err
	}

	dst := image.NewRGBA(img.Bounds())
	draw.Copy(dst, image.ZP, img, img.Bounds(), draw.Src, nil)

	// the complete excuse
	if err := drawString(`"`+long+`"`, size, &longTextBB, dst); err != nil {
		return "", err
	}

	// the short excuse
	if err := drawString(short, size-2.0, &shortTextBB, dst); err != nil {
		return "", err
	}

	// save
	td := os.TempDir()
	outFile, err := ioutil.TempFile(td, "excuse")
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	stat, err := outFile.Stat()
	if err != nil {
		return "", err
	}
	fname := filepath.Join(td, stat.Name())
	defer os.Remove(fname)
	b := bufio.NewWriter(outFile)
	if err = png.Encode(outFile, dst); err != nil {
		return "", err
	}
	if err = b.Flush(); err != nil {
		return "", err
	}

	n, err := uploader.Upload(fname)
	if nil != err {
		return "", err
	}

	return n, nil
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
	}

	bbWidth := bb.Max.X - bb.Min.X
	bbMiddle := bb.Min.X + bbWidth/2
	textStart := fixed.I(bbMiddle) - adv/2

	return size, textStart
}

type ImgurUploader struct {
}

type ImgurAnswer struct {
	Data struct {
		ID          string        `json:"id"`
		Title       interface{}   `json:"title"`
		Description interface{}   `json:"description"`
		Datetime    int           `json:"datetime"`
		Type        string        `json:"type"`
		Animated    bool          `json:"animated"`
		Width       int           `json:"width"`
		Height      int           `json:"height"`
		Size        int           `json:"size"`
		Views       int           `json:"views"`
		Bandwidth   int           `json:"bandwidth"`
		Vote        interface{}   `json:"vote"`
		Favorite    bool          `json:"favorite"`
		Nsfw        interface{}   `json:"nsfw"`
		Section     interface{}   `json:"section"`
		AccountURL  interface{}   `json:"account_url"`
		AccountID   int           `json:"account_id"`
		IsAd        bool          `json:"is_ad"`
		InMostViral bool          `json:"in_most_viral"`
		Tags        []interface{} `json:"tags"`
		AdType      int           `json:"ad_type"`
		AdURL       string        `json:"ad_url"`
		InGallery   bool          `json:"in_gallery"`
		Deletehash  string        `json:"deletehash"`
		Name        string        `json:"name"`
		Link        string        `json:"link"`
	} `json:"data"`
	Success bool `json:"success"`
	Status  int  `json:"status"`
}

func NewImgurUploader() *ImgurUploader {
	return &ImgurUploader{}
}

func (iu *ImgurUploader) Upload(fname string) (string, error) {
	resp, err := goimgur.UploadImage(fname)
	if nil != err {
		return "", err
	}
	defer resp.Body.Close()
	var res ImgurAnswer
	err = json.NewDecoder(resp.Body).Decode(&res)
	if nil != err {
		return "", err
	}
	if !res.Success {
		return "", errors.Errorf("error when uploading image to imgur: %d", res.Status)
	}
	return res.Data.Link, nil
}

func generateImgur(cache Cacher, uploader Uploader) func(rw http.ResponseWriter, req *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)
		short := vars["short"]
		long := vars["long"]
		key := Key{Short: short, Long: long}
		url, has := cache.Get(key)
		if !has {
			uUrl, err := create(uploader, short, long)
			if nil != err {
				log.Printf("error happened: %s", err.Error())
				rw.WriteHeader(500)
				return
			}
			cache.Set(key, uUrl)
			url = uUrl
		}
		http.Redirect(rw, req, url, http.StatusTemporaryRedirect)
	}
}

func NewInMemoryCache() *InMemoryCache {
	return &InMemoryCache{
		images: make(map[Key]string),
		mutex:  &sync.RWMutex{},
	}
}

func (c *InMemoryCache) Set(key Key, url string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.images[key] = url
}

func (c *InMemoryCache) Get(key Key) (string, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	s, ok := c.images[key]
	return s, ok
}

func main() {
	flag.Parse()
	cache := NewInMemoryCache()
	uploader := NewImgurUploader()

	goimgur.ClientID = *clientId
	r := mux.NewRouter()
	r.HandleFunc("/{short:[a-zA-Z0-9 !]+}/{long:[a-zA-Z0-9 !]+}", generateImgur(cache, uploader))
	log.Printf("running on port: %d", *port)
	http.ListenAndServe(":"+strconv.Itoa(*port), r)
}

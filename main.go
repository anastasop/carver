package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/gomono"
)

var (
	fg   *image.Uniform // the font color
	bg   *image.Uniform // the background color
	font *truetype.Font // the font to use
)

var fgColor = flag.String("c", "000000ff", "Foreground color NRGBA in 8 small hex digits. Ex 0a0b0cff.")
var bgColor = flag.String("b", "ffffe0ff", "Background color NRGBA in 8 small hex digits. Ex 0a0b0cff.")
var outFile = flag.String("o", "", "Output image file.")
var inFile = flag.String("i", "", "Input image file to use as canvas. If not given, render over the background color.")
var text = flag.String("t", "", "Text to render. If empty, read stdin.")
var dpi = flag.Float64("d", 96.0, "dpi")
var fontSize = flag.Float64("p", 11.0, "Font size in points.")
var fontFile = flag.String("f", "", "A TTF file. If empty use gomono https://blog.golang.org/go-fonts.")
var report = flag.Bool("n", false, "Don't render text but estimate and print the bounds.")
var anchor = flag.String("a", "tl", "Where to place text on a 3x3 grid. One of tl, tc, tr, cl, c, cr, bl, bc, br. (tl -> top left etc)")

// allocColorImage parses col which is an NRGBA color (ex 0a0b0cff) and return a uniform image of that color
func allocColorImage(col string) *image.Uniform {
	var c uint32
	fmt.Sscanf(col, "%x", &c)

	return image.NewUniform(color.NRGBA{
		R: uint8((c >> 24) & 0xFF),
		G: uint8((c >> 16) & 0xFF),
		B: uint8((c >> 8) & 0xFF),
		A: uint8(c & 0xFF),
	})
}

// bounds estimates an upper bound for the area needed to render lines
// The ctx must be configured with fontsize and DPI
func bounds(ctx *freetype.Context, lines []string) image.Rectangle {
	maxLen := 0
	for _, line := range lines {
		if l := len(line); l > maxLen {
			maxLen = l
		}
	}

	po := ctx.PointToFixed(*fontSize)       // 1em for all margins top, bottom, left, right
	vs := ctx.PointToFixed(*fontSize + 2.0) // vertical spacing between lines need 2 more points

	dx := maxLen*po.Ceil() + 4*po.Ceil()     // +4 for left, right margins and a safety margin
	dy := len(lines)*vs.Ceil() + 4*po.Ceil() // +4 po for top, bottom margins and a safety margin

	return image.Rect(0, 0, dx, dy)
}

// render creates a new image with a transparent background, renders the lines and returns it
func render(lines []string) (image.Image, error) {
	ctx := freetype.NewContext()
	ctx.SetFont(font)
	ctx.SetFontSize(*fontSize)
	ctx.SetSrc(fg)
	ctx.SetDPI(*dpi)

	// bounds needs to be called after SetDPI, SetFont, SetFontSize
	img := image.NewRGBA(bounds(ctx, lines))
	ctx.SetClip(img.Bounds())
	ctx.SetDst(img)

	vs := ctx.PointToFixed(*fontSize + 2.0) // vertical spacing between lines need 2 more points
	offset := freetype.Pt(16, 16)           // 16 pixels fixed size margins
	bounds := freetype.Pt(0, 0)             // actual bounds of image, updated after each draw operation
	p := offset
	for _, line := range lines {
		p.Y += vs
		if p1, err := ctx.DrawString(line, p); err != nil {
			return nil, err
		} else if p1.X > bounds.X {
			bounds.X = p1.X
		}
		bounds.Y = p.Y
	}
	bounds.X += offset.X // add right margin
	bounds.Y += offset.Y // add bottom margin

	return img.SubImage(image.Rect(0, 0, bounds.X.Ceil(), bounds.Y.Ceil())), nil
}

// writeImage write the PNG encoding of img to the new file fname
func writeImage(img image.Image, fname string) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}

	return ioutil.WriteFile(fname, buf.Bytes(), 0644)
}

// textToRender reads standard input or the -t flag, optionally replaces tabs with spaces and returns the lines
func textToRender() ([]string, error) {
	lines := make([]string, 0)

	var scanner *bufio.Scanner
	if *text == "" {
		scanner = bufio.NewScanner(os.Stdin)
	} else {
		scanner = bufio.NewScanner(bytes.NewBufferString(*text))
	}
	for scanner.Scan() {
		lines = append(lines, strings.ReplaceAll(scanner.Text(), "\t", "    "))
	}

	return lines, scanner.Err()
}

// canvas returns the image to write on. It is either a uniform background color or an image read from a file.
// If fname is not empty it reads the image file and returns it. Otherwise it allocates an image of size bounds,
// uniformly colored with the background color
func canvas(fname string, bounds image.Rectangle) (draw.Image, error) {
	if fname == "" {
		dst := image.NewRGBA(bounds)
		draw.Draw(dst, dst.Bounds(), bg, image.Pt(0, 0), draw.Src)
		return dst, nil
	}

	fin, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer fin.Close()

	img, _, err := image.Decode(fin)
	if err != nil {
		log.Fatal(err)
	}

	return img.(draw.Image), nil
}

// textRect returns the rectangle of dst where src should be placed according to pos
// Pos is one of tl, tc, tr, cl, c, cr, bl, bc, br and correspond to the 9 squares of the 3x3 grid
func textRect(dst, src image.Image, pos string) image.Rectangle {
	ox := dst.Bounds().Max.X - src.Bounds().Dx()
	oy := dst.Bounds().Max.Y - src.Bounds().Dy()

	var pt image.Point
	switch pos {
	case "tl":
		pt = image.Pt(0, 0)
	case "tc":
		pt = image.Pt(ox/2, 0)
	case "tr":
		pt = image.Pt(ox, 0)
	case "cl":
		pt = image.Pt(0, oy/2)
	case "c":
		pt = image.Pt(ox/2, oy/2)
	case "cr":
		pt = image.Pt(ox, oy/2)
	case "bl":
		pt = image.Pt(0, oy)
	case "bc":
		pt = image.Pt(ox/2, oy)
	case "br":
		pt = image.Pt(ox, oy)
	default:
		usage()
	}
	ap := dst.Bounds().Min.Add(pt)

	return image.Rect(ap.X, ap.Y, ap.X+src.Bounds().Dx(), ap.Y+src.Bounds().Dy())
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage: carver -t <text> -i image.png -o out.png

Carver renders lines of text over a png or jpeg image.
`)
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("")
	flag.Usage = usage
	flag.Parse()

	if *outFile == "" && !*report {
		usage()
	}

	var fontData []byte
	if *fontFile == "" {
		fontData = gomono.TTF
	} else {
		if data, err := os.ReadFile(*fontFile); err != nil {
			log.Fatal(err)
		} else {
			fontData = data
		}
	}

	f, err := freetype.ParseFont(fontData)
	if err != nil {
		log.Fatal(err)
	}
	font = f

	fg = allocColorImage(*fgColor)
	bg = allocColorImage(*bgColor)

	lines, err := textToRender()
	if err != nil {
		log.Fatal(err)
	}

	img, err := render(lines)
	if err != nil {
		log.Fatal(err)
	}

	if *report {
		fmt.Printf("%dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	} else {
		cimg, err := canvas(*inFile, img.Bounds())
		if err != nil {
			log.Fatal(err)
		}

		draw.Draw(cimg, textRect(cimg, img, *anchor), img, image.Pt(0, 0), draw.Over)

		err = writeImage(cimg, *outFile)
		if err != nil {
			log.Fatal(err)
		}
	}
}

package main

import (
	"fmt"
	"github.com/DavidGamba/go-getoptions"
	"github.com/raspi/heksa/pkg/color"
	"github.com/raspi/heksa/pkg/iface"
	"github.com/raspi/heksa/pkg/reader"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
)

var VERSION = `v0.0.0`
var BUILD = `dev`
var BUILDDATE = `0000-00-00T00:00:00+00:00`

const AUTHOR = `Pekka Järvinen`
const HOMEPAGE = `https://github.com/raspi/heksa`

// Parse command line arguments
func getParams() (source iface.ReadSeekerCloser, displays []reader.ByteFormatter, offsetViewer []reader.OffsetFormatter, limit uint64, palette [256]color.AnsiColor, filesize int64) {
	opt := getoptions.New()

	opt.HelpSynopsisArgs(`<filename> or STDIN`)

	opt.Bool(`help`, false,
		opt.Alias("h", "?"),
		opt.Description(`Show this help`),
	)

	opt.Bool(`version`, false,
		opt.Description(`Show version information`),
	)

	argOffset := opt.StringOptional(`offset-format`, `hex`,
		opt.Alias(`o`),
		opt.ArgName(`fmt1[,fmt2]`),
		opt.Description(
			`One or two of: `+strings.Join(reader.GetOffsetViewerList(), `, `)+`, no, ''.`+
				"\n"+
				`First one is displayed on the left side and second one on right side after formatters.`,
		),
	)

	argFormat := opt.StringOptional(`format`, `hex,asc`,
		opt.Alias(`f`),
		opt.ArgName(`fmt1,fmt2,..`),
		opt.Description(`One or multiple of: `+strings.Join(reader.GetViewerList(), `, `)),
	)

	argLimit := opt.StringOptional(`limit`, `0`,
		opt.Alias("l"),
		opt.ArgName(`[prefix]bytes`),
		opt.Description(`Read only N bytes (0 = no limit). See NOTES.`),
	)

	argSeek := opt.StringOptional(`seek`, `0`,
		opt.Alias("s"),
		opt.ArgName(`[prefix]offset`),
		opt.Description(`Start reading from certain offset. See NOTES.`),
	)

	remainingArgs, err := opt.Parse(os.Args[1:])

	if opt.Called("help") {
		fmt.Fprintf(os.Stdout, `heksa - hex file dumper %v - (%v)`+"\n", VERSION, BUILDDATE)
		fmt.Fprintf(os.Stdout, `(c) %v 2019- [ %v ]`+"\n", AUTHOR, HOMEPAGE)
		fmt.Fprintln(os.Stdout, opt.Help())
		fmt.Fprintln(os.Stdout, `NOTES:`)
		fmt.Fprintln(os.Stdout, `    - You can use prefixes for seek and limit. 0x = hex, 0b = binary, 0o = octal`)
		fmt.Fprintln(os.Stdout, `    - Use 'no' or '' for offset formatter for disabling offset output`)
		fmt.Fprintln(os.Stdout, `    - Use '--seek \-[prefix]1000' for seeking to end of file`)
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, `EXAMPLES:`)
		fmt.Fprintln(os.Stdout, `    heksa -f hex,asc,bit foo.dat`)
		fmt.Fprintln(os.Stdout, `    heksa -o hex,per -f hex,asc foo.dat`)
		fmt.Fprintln(os.Stdout, `    heksa -o hex -f hex,asc,bit foo.dat`)
		fmt.Fprintln(os.Stdout, `    heksa -o no -f bit foo.dat`)
		fmt.Fprintln(os.Stdout, `    heksa -l 0x1024 foo.dat`)
		fmt.Fprintln(os.Stdout, `    heksa -s 0b1010 foo.dat`)
		os.Exit(0)
	} else if opt.Called("version") {
		fmt.Fprintf(os.Stdout, `%v build %v on %v`+"\n", VERSION, BUILD, BUILDDATE)
		os.Exit(0)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n\n", err)
		fmt.Fprintln(os.Stderr, opt.Help(getoptions.HelpSynopsis))
		os.Exit(1)
	}

	limit, err = strconv.ParseUint(*argLimit, 0, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, `error parsing limit: %v`, err)
		os.Exit(1)
	}

	startOffset, err := strconv.ParseInt(strings.Replace(*argSeek, `\`, ``, -1), 0, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, `error parsing seek: %v`, err)
		os.Exit(1)
	}

	offsetViewer, err = reader.GetOffsetFormatters(strings.Split(*argOffset, `,`))
	if err != nil {
		fmt.Fprintf(os.Stderr, `error getting offset formatter: %v`, err)
		os.Exit(1)
	}

	displays, err = reader.GetViewers(strings.Split(*argFormat, `,`))
	if err != nil {
		fmt.Fprintf(os.Stderr, `error getting formatter: %v`, err)
		os.Exit(1)
	}

	palette = defaultCharacterColors

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Stdin has data
		source = os.Stdin
		filesize = -1
	} else {
		// Read file
		if len(remainingArgs) != 1 {
			fmt.Fprintln(os.Stderr, `error: no file given as argument, see --help`)
			os.Exit(1)
		}

		fpath := remainingArgs[0]

		fhandle, err := os.Open(fpath)
		if err != nil {
			fmt.Fprintf(os.Stderr, `error opening file: %v`, err)
			os.Exit(1)
		}

		fi, err := fhandle.Stat()
		if err != nil {
			fmt.Fprintf(os.Stderr, `error stat'ing file: %v`, err)
			os.Exit(1)
		}

		if fi.IsDir() {
			fmt.Fprintf(os.Stderr, `error: %v is directory`, fpath)
			os.Exit(1)
		}

		// Seek to given offset
		if startOffset > 0 {
			_, err = fhandle.Seek(startOffset, io.SeekCurrent)
		} else if startOffset < 0 {
			_, err = fhandle.Seek(startOffset, io.SeekEnd)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, `couldn't seek to %v: %v`, startOffset, err)
			os.Exit(1)
		}

		filesize = fi.Size()
		source = fhandle
	}

	return source, displays, offsetViewer, limit, palette, filesize
}

func main() {
	source, displays, offViewer, limit, palette, filesize := getParams()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	r := reader.New(source, offViewer, displays, palette, filesize)

	isEven := false
	// Dump hex
	for {
		select {
		case <-stop: // Kill or ctrl-C
			break
		default:
		}

		s, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}

			fmt.Fprintln(os.Stderr, fmt.Sprintf(`error while reading file: %v`, err))
			os.Exit(1)
		}

		color := LineEven
		if isEven {
			color = LineOdd
		}
		isEven = !isEven

		fmt.Printf(`%s%s%s`+"\n", color, s, clear)

		if limit > 0 && r.ReadBytes >= limit {
			// Limit is set and found
			break
		}

	}

	source.Close()

}

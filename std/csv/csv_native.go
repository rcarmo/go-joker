package csv

import (
	"encoding/csv"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"
	"strings"
	"unicode/utf8"

	. "github.com/rcarmo/go-joker/core"
)

func csvDelimiter(obj coretypes.Object, name string) rune {
	r := EnsureObjectIsChar(obj, name+": %s").Ch
	if r == '\n' || r == '\r' || r == 0 || r == utf8.RuneError {
		panic(RT.NewError("csv/" + name + " must be a valid delimiter"))
	}
	return r
}

func csvLazySeq(rdr *csv.Reader) *LazySeq {
	var c = func(args []coretypes.Object) coretypes.Object {
		t, err := rdr.Read()
		if err == io.EOF {
			return EmptyList
		}
		PanicOnErr(err)
		return NewConsSeq(MakeStringVector(t), csvLazySeq(rdr))
	}
	return NewLazySeq(Proc{Fn: c})
}

func csvSeqOpts(src coretypes.Object, opts Map) coretypes.Object {
	var rdr io.Reader
	switch src := src.(type) {
	case coretypes.String:
		rdr = strings.NewReader(src.S)
	case io.Reader:
		rdr = src
	default:
		panic(RT.NewError("src must be a string or io.Reader"))
	}
	csvReader := csv.NewReader(rdr)
	csvReader.ReuseRecord = true
	if ok, c := opts.Get(MakeKeyword("comma")); ok {
		csvReader.Comma = csvDelimiter(c, "comma")
	}
	if ok, c := opts.Get(MakeKeyword("comment")); ok {
		csvReader.Comment = csvDelimiter(c, "comment")
		if csvReader.Comment == csvReader.Comma {
			panic(RT.NewError("csv/comment must differ from comma"))
		}
	}
	if ok, c := opts.Get(MakeKeyword("fields-per-record")); ok {
		csvReader.FieldsPerRecord = EnsureObjectIsInt(c, "fields-per-record: %s").I
	}
	if ok, c := opts.Get(MakeKeyword("lazy-quotes")); ok {
		csvReader.LazyQuotes = EnsureObjectIsBoolean(c, "lazy-quotes: %s").B
	}
	if ok, c := opts.Get(MakeKeyword("trim-leading-space")); ok {
		csvReader.TrimLeadingSpace = EnsureObjectIsBoolean(c, "trim-leading-space: %s").B
	}
	return csvLazySeq(csvReader)
}

func sliceOfStrings(obj coretypes.Object) (res []string) {
	s := EnsureObjectIsSeqable(obj, "CSV record: %s").Seq()
	for !s.IsEmpty() {
		res = append(res, s.First().ToString(false))
		s = s.Rest()
	}
	return
}

func writeWriter(wr io.Writer, data coretypes.Seqable, opts Map) {
	csvWriter := csv.NewWriter(wr)
	if ok, c := opts.Get(MakeKeyword("comma")); ok {
		csvWriter.Comma = csvDelimiter(c, "comma")
	}
	if ok, c := opts.Get(MakeKeyword("use-crlf")); ok {
		csvWriter.UseCRLF = EnsureObjectIsBoolean(c, "use-crlf: %s").B
	}
	s := data.Seq()
	for !s.IsEmpty() {
		err := csvWriter.Write(sliceOfStrings(s.First()))
		PanicOnErr(err)
		s = s.Rest()
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		panic(RT.NewError("csv/write: " + err.Error()))
	}
}

func write(wr io.Writer, data coretypes.Seqable, opts Map) coretypes.Object {
	writeWriter(wr, data, opts)
	return NIL
}

func writeString(data coretypes.Seqable, opts Map) string {
	var b strings.Builder
	writeWriter(&b, data, opts)
	return b.String()
}

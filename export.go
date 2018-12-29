package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/aaronland/go-artisanal-integers"
	brooklyn_integers "github.com/aaronland/go-brooklynintegers-api"
	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
	"github.com/whosonfirst/go-whosonfirst-uri"	
	"github.com/whosonfirst/go-whosonfirst-readwrite/writer"
	"io/ioutil"
	_ "log"
	"time"
)

type Feature struct {
	Type       string      `json:"type"`
	Id         int64       `json:"id"`
	Properties interface{} `json:"properties"`
	Bbox       interface{} `json:"bbox,omitempty"`
	Geometry   interface{} `json:"geometry"`
}

type ExportOptions struct {
	Strict        bool
	IntegerClient artisanalinteger.Client
}

func DefaultExportOptions() (*ExportOptions, error) {

	bi_client := brooklyn_integers.NewAPIClient()

	opts := ExportOptions{
		Strict:        true,
		IntegerClient: bi_client,
	}

	return &opts, nil
}

func ExportFeatureToWriter(feature interface{}, wr writer.Writer, opts *ExportOptions) (int64, string, error) {

	body, err := json.Marshal(feature)

	if err != nil {
		return -1, "", err
	}

	return ExportToWriter(body, wr, opts)
}

func ExportFeature(feature interface{}, opts *ExportOptions) ([]byte, error) {

	body, err := json.Marshal(feature)

	if err != nil {
		return nil, err
	}

	return Export(body, opts)
}

func ExportToWriter(body []byte, wr writer.Writer, opts *ExportOptions) (int64, string, error) {

	pretty, err := Export(body, opts)

	if err != nil {
		return -1, "", err
	}

	rsp := gjson.GetBytes(pretty, "properties.wof:id")

	if !rsp.Exists() {
		return -1, "", errors.New("Missing wof:id")
	}

	wof_id := rsp.Int()

	path, err := uri.Id2RelPath(wof_id)

	if err != nil {
		return -1, "", err
	}

	r := bytes.NewReader(pretty)
	fh := ioutil.NopCloser(r)

	err = wr.Write(path, fh)

	if err != nil {
		return -1, "", err
	}
	
	return wof_id, wr.URI(path), nil
}

func Export(feature []byte, opts *ExportOptions) ([]byte, error) {

	var err error

	feature, err = Prepare(feature, opts)

	if err != nil {
		return nil, err
	}

	feature, err = Format(feature, opts)

	if err != nil {
		return nil, err
	}

	return feature, nil
}

func Prepare(feature []byte, opts *ExportOptions) ([]byte, error) {

	var err error

	now := int32(time.Now().Unix())

	created := gjson.GetBytes(feature, "properties.wof:created")

	if !created.Exists() {

		feature, err = sjson.SetBytes(feature, "properties.wof:created", now)

		if err != nil {
			return nil, err
		}
	}

	feature, err = sjson.SetBytes(feature, "properties.wof:lastmodified", now)

	if err != nil {
		return nil, err
	}

	var wof_id int64

	rsp := gjson.GetBytes(feature, "properties.wof:id")

	if rsp.Exists() {

		wof_id = rsp.Int()

	} else {

		i, err := opts.IntegerClient.NextInt()

		if err != nil {
			return nil, err
		}

		wof_id = i

		feature, err = sjson.SetBytes(feature, "properties.wof:id", wof_id)

		if err != nil {
			return nil, err
		}
	}

	id := gjson.GetBytes(feature, "id")

	if !id.Exists() {

		feature, err = sjson.SetBytes(feature, "id", wof_id)

		if err != nil {
			return nil, err
		}

	}

	return feature, nil
}

func Format(feature []byte, opts *ExportOptions) ([]byte, error) {

	// see also:
	// https://github.com/tidwall/pretty/issues/2
	// https://gist.github.com/tidwall/ca6ca1dd0cb780f0be4d134f8e4eb7bc

	// the first thing we need to do is ensure that top-level keys
	// are sorted properly (see Feature definition above) specifically
	// so that the bbox and geometry properties are at the end of the
	// file

	var f Feature
	json.Unmarshal(feature, &f)

	// this has the side-effect of ensuring that all the keys in the
	// properties dictionary are sorted automagically

	feature, err := json.Marshal(f)

	if err != nil {
		return nil, err
	}

	// get the geometry (that will be mutated)

	geom_m := gjson.GetBytes(feature, "geometry")

	// sanity checks

	if geom_m.Index == 0 || len(geom_m.Raw) == 0 || geom_m.Raw[0] != '{' || geom_m.Raw[len(geom_m.Raw)-1] != '}' {
		// probably a generic geometry, make it ugly
		return pretty.Ugly(feature), nil
	}

	// make an ugly copy of just the geometry segment
	geom := pretty.Ugly(feature[geom_m.Index : geom_m.Index+len(geom_m.Raw)])

	// now for the tricky part.

	// empty the geometry object.
	// Note that this mutates the original feature.

	for i := 1; i < len(geom_m.Raw)-1; i++ {
		feature[geom_m.Index+i] = ' '
	}

	// make the json pretty

	feature = pretty.Pretty(feature)

	// find the new location of the geometry

	geom_m = gjson.GetBytes(feature, "geometry")

	// allocate the space for the final json.

	final := make([]byte, len(feature)+len(geom)-2)

	copy(final, feature[:geom_m.Index])
	copy(final[geom_m.Index:], geom)
	copy(final[geom_m.Index+len(geom):], feature[geom_m.Index+len(geom_m.Raw):])

	return final, nil
}

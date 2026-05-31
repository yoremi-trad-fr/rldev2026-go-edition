package g00

import (
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type metadataXML struct {
	XMLName xml.Name    `xml:"vas_g00"`
	Format  string      `xml:"format,attr,omitempty"`
	Regions []regionXML `xml:"regions>region"`
}

type regionXML struct {
	X1     int        `xml:"x1,attr"`
	Y1     int        `xml:"y1,attr"`
	X2     int        `xml:"x2,attr"`
	Y2     int        `xml:"y2,attr"`
	Origin *originXML `xml:"origin,omitempty"`
	Parts  []partXML  `xml:"part,omitempty"`
}

type originXML struct {
	X int `xml:"x,attr"`
	Y int `xml:"y,attr"`
}

type partXML struct {
	X     int `xml:"x,attr"`
	Y     int `xml:"y,attr"`
	W     int `xml:"w,attr"`
	H     int `xml:"h,attr"`
	Trans int `xml:"trans,attr"`
}

// ReadMetadataFile reads a vas_g00 XML metadata file.
func ReadMetadataFile(path string) (format int, regions []Region, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, nil, err
	}
	var meta metadataXML
	if err := xml.Unmarshal(data, &meta); err != nil {
		return 0, nil, err
	}
	if meta.XMLName.Local != "vas_g00" {
		return 0, nil, fmt.Errorf("g00 metadata: expected <vas_g00>, got <%s>", meta.XMLName.Local)
	}
	format = 0
	if strings.TrimSpace(meta.Format) != "" {
		format, err = strconv.Atoi(strings.TrimSpace(meta.Format))
		if err != nil {
			return 0, nil, fmt.Errorf("g00 metadata: invalid format %q", meta.Format)
		}
	}
	regions = make([]Region, 0, len(meta.Regions))
	for _, xr := range meta.Regions {
		r := Region{X1: xr.X1, Y1: xr.Y1, X2: xr.X2, Y2: xr.Y2}
		if xr.Origin != nil {
			r.OriginX = xr.Origin.X
			r.OriginY = xr.Origin.Y
		}
		for _, xp := range xr.Parts {
			r.Parts = append(r.Parts, Part{
				X: xp.X, Y: xp.Y, Width: xp.W, Height: xp.H, Trans: xp.Trans,
			})
		}
		regions = append(regions, r)
	}
	return format, regions, nil
}

// WriteMetadataFile writes a vas_g00 XML metadata file for format 2 regions.
func WriteMetadataFile(path string, img *Image) error {
	regions := img.Regions
	if len(regions) == 0 && img.Format == 2 {
		regions = []Region{defaultRegion(img.Width, img.Height)}
	}
	meta := metadataXML{Format: strconv.Itoa(img.Format)}
	for _, r := range regions {
		xr := regionXML{X1: r.X1, Y1: r.Y1, X2: r.X2, Y2: r.Y2}
		if r.OriginX != 0 || r.OriginY != 0 {
			xr.Origin = &originXML{X: r.OriginX, Y: r.OriginY}
		}
		meta.Regions = append(meta.Regions, xr)
	}
	out, err := xml.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	out = append([]byte(xml.Header), out...)
	out = append(out, '\n')
	return os.WriteFile(path, out, 0644)
}

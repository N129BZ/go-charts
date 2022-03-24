package tafs

import (
	"log"
	"fmt"
	"encoding/json"
	"encoding/xml"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"time"
)

type Response struct {
	XMLName      xml.Name   `xml:"response" json:"-"`
	Version      string     `xml:"version,attr"`
	RequestIndex int32      `xml:"request_index"`
	Errors       []string   `xml:"errors>error"`
	Warnings     []string   `xml:"warnings>warning"`
	TimeTakenMs  int32      `xml:"time_taken_ms"`
	DataSource   DataSource `xml:"data_source"`
	Request      Request    `xml:"request"`
	Data         Data       `xml:"data"`
}

type Request struct {
	XMLName xml.Name `xml:"request" json:"-"`
	Type    string   `xml:"type,attr"`
}

type DataSource struct {
	XMLName xml.Name `xml:"data_source" json:"-"`
	Name    string   `xml:"name,attr"`
}

type Data struct {
	XMLName    xml.Name `xml:"data" json:"-"`
	NumResults int32    `xml:"num_results,attr"`
	Tafs        []Taf   `xml:"TAF"`
}

type SkyCondition struct {
	XMLName        xml.Name `xml:"sky_condition" json:"-"`
	SkyCover       string   `xml:"sky_cover,attr"`
	CloudBaseFtAGL int32    `xml:"cloud_base_ft_agl,attr"`
	CloudType      string   `xml:"cloud_type,attr"`
}

type TurbulenceCondition struct {
	XMLName     xml.Name `xml:"turbulence_condition" json:"-"`
	Intensity   string   `xml:"turbulence_intensity,attr"`
	MinAltFtAGL int32    `xml:"turbulence_min_alt_ft_agl,attr"`
	MaxAltFtAGL int32    `xml:"turbulence_max_alt_ft_agl,attr"`
}

type IcingCondition struct {
	XMLName     xml.Name `xml:"icing_condition" json:"-"`
	Intensity   string   `xml:"icing_intensity,attr"`
	MinAltFtAGL int32    `xml:"icing_min_alt_ft_agl,attr"`
	MaxAltFtAGL int32    `xml:"icing_max_alt_ft_agl,attr"`
}

type Temperature struct {
	ValidTime    time.Time `xml:"valid_time"`
	SurfaceTempC float64   `xml:"sfc_temp_c"`
	MaxTempC     string    `xml:"max_temp_c"`
	MinTempC     string    `xml:"min_temp_c"`
}

type Forecast struct {
	XMLName             xml.Name              `xml:"forecast" json:"-"`
	FcstTimeFrom        time.Time             `xml:"fcst_time_from"`
	FcstTimeTo          time.Time             `xml:"fcst_time_to"`
	ChangeIndicator     string                `xml:"change_indicator"`
	TimeBecoming        time.Time             `xml:"time_becoming"`
	Probability         int32                 `xml:"probability"`
	WindDirDegrees      int16                 `xml:"wind_dir_degrees"`
	WindSpeedKt         int32                 `xml:"wind_speed_kt"`
	WindGustKt          int32                 `xml:"wind_gust_kt"`
	WindShearHgtFtAgl   int16                 `xml:"wind_shear_hgt_ft_agl"`
	WindShearDirDegrees int16                 `xml:"wind_shear_dir_degrees"`
	WindShearSpeedKt    float64               `xml:"wind_shear_speed_kt"`
	VisibilityStatuteMi float64               `xml:"visibility_statute_mi"`
	AltimInHg           float64               `xml:"altim_in_hg"`
	VertVisFt           int16                 `xml:"vert_vis_ft"`
	WxString            string                `xml:"wx_string"`
	NotDecoded          string                `xml:"not_decoded"`
	SkyCondition        []SkyCondition        `xml:"sky_condition"`
	TurbulenceCondition []TurbulenceCondition `xml:"turbulence_condition"`
	IcingCondition      []IcingCondition      `xml:"icing_condition"`
	Temperature         Temperature           `xml:"temperature"`
	ValidTime           string                `xml:"valid_time"`
	SfcTempC            float64               `xml:"sfc_temp_c"`
	MaxTempC            float64               `xml:"max_temp_c"`
	MinTempC            float64               `xml:"min_temp_c"`
}

type Taf struct {
	XMLName       xml.Name   `xml:"TAF" json:"-"`
	RawText       string     `xml:"raw_text"`
	StationId     string     `xml:"station_id"`
	IssueTime     time.Time  `xml:"issue_time"`
	BulletinTime  time.Time  `xml:"bulletin_time"`
	ValidTimeFrom time.Time  `xml:"valid_time_from"`
	ValidTimeTo   time.Time  `xml:"valid_time_to"`
	Remarks       string     `xml:"remarks"`
	Latitude      float64    `xml:"latitude"`
	Longitude     float64    `xml:"longitude"`
	ElevationM    float64    `xml:"elevation_m"`
	Forecast      []Forecast `xml:"forecast"`
}

// SaveAsJSONFile downloads xml file from ADDS weather server and converts to a JSON string
func (r *Response) SaveAsJSONFile(url string) (err error) {
	
	t := &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   60 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 60 * time.Second,
	}
	c := &http.Client{
		Transport: t,
	}
	resp, err := c.Get(url)
	if err != nil {
		return err
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Error status code received: %v", resp.StatusCode)
	}

	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = xml.Unmarshal(data, &r)
	if err != nil {
		return err
	}

	var s string
	s, err = r.ToJson()
	if err != nil {
		return err
	}

	jsonfile := "./tafs.json"
	err = os.Remove(jsonfile)
	if err != nil {
		log.Println(err)
	}

	nstr := []byte("{ \"tafs\": " + s + "}")
	err = os.WriteFile(jsonfile, nstr, 0644)
	if err != nil {
		return err
	}

	return nil
}

func (r *Response) ToRawTextOnly() (s []string) {
	for _, taf := range r.Data.Tafs {
		s = append(s, taf.RawText)
	}
	return
}

func (r *Response) ToJson() (s string, err error) {
	b, err := json.Marshal(r.Data.Tafs)
	if err != nil {
		return "", err
	}
	s = string(b)
	return
}

func (r *Response) ToJsonIndented() (s string, err error) {
	b, err := json.MarshalIndent(r.Data.Tafs, "", "  ")
	if err != nil {
		return "", err
	}
	s = string(b)
	return
}
	
package pireps

import (
	"log"
	"fmt"
	"net"
	"net/http"
	"encoding/json"
	"encoding/xml"
	"io/ioutil"
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
	Pireps     []Pirep  `xml:"PIREP"`
}

type SkyCondition struct {
	Text           xml.Name `xml:"sky_condition" json:"-"`
	SkyCover       string `xml:"sky_cover,attr"`
	CloudBaseFtMsl string `xml:"cloud_base_ft_msl,attr"`
	CloudTopFtMsl  string `xml:"cloud_top_ft_msl,attr"`
}

type TurbulenceCondition struct {
	Text                xml.Name `xml:"turbulence_condition" json:"-"`
	TurbulenceType      string `xml:"turbulence_type,attr"`
	TurbulenceIntensity string `xml:"turbulence_intensity,attr"`
	TurbulenceBaseFtMsl string `xml:"turbulence_base_ft_msl,attr"`
	TurbulenceFreq      string `xml:"turbulence_freq,attr"`
	TurbulenceTopFtMsl  string `xml:"turbulence_top_ft_msl,attr"`
}

type IcingCondition struct {
	XMLName        xml.Name `xml:"icing_condition" json:"-"`
	IcingIntensity string `xml:"icing_intensity,attr"`
	IcingBaseFtMsl string `xml:"icing_base_ft_msl,attr"`
	IcingTopFtMsl  string `xml:"icing_top_ft_msl,attr"`
	IcingType      string `xml:"icing_type,attr"`
}

type QualityControlFlags struct {
	XMLName         xml.Name `xml:"quality_control_flags" json:"-"`
	BadLocation     string `xml:"bad_location"`
	MidPointAssumed string `xml:"mid_point_assumed"`
}

type Pirep struct {
	XMLName             xml.Name `xml:"PIREP" json:"-"`
	RawText             string   `xml:"raw_text"`
	ReceiptTime         time.Time `xml:"receipt_time"`
	ObservationTime     time.Time `xml:"observation_time"`
	AircraftRef         string `xml:"aircraft_ref"`
	Latitude            float64 `xml:"latitude"`
	Longitude           float64 `xml:"longitude"`
	AltitudeFtMsl       float64 `xml:"altitude_ft_msl"`
	TempC               float64 `xml:"temp_c"`
	WindDirDegrees      int32   `xml:"wind_dir_degrees"`
	WindSpeedKt         int32   `xml:"wind_speed_kt"`
	PirepType           string  `xml:"pirep_type"`
	QualityControlFlags QualityControlFlags
	SkyCondition        []SkyCondition
	TurbulenceCondition []TurbulenceCondition
	WxString            string `xml:"wx_string"`
	IcingCondition      []IcingCondition
	VisibilityStatuteMi float64 `xml:"visibility_statute_mi"`
}

// SaveAsJSONFile downloads xml file from ADDS weather server and converts to a JSON string
func (r *Response) SaveAsJSONFile(url string) (err error) {

	t := &http.Transport{
		Dial: (&net.Dialer{
				Timeout: 60 * time.Second,
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

	jsonfile := "./pireps.json"
	err = os.Remove(jsonfile)
	if err != nil {
		log.Println(err)
	}

	nstr := []byte("{ \"pireps\": " + s + "}")
	err = os.WriteFile(jsonfile, nstr, 0644)
	if (err != nil) {
		return err
	}
	
	return nil
}

func (r *Response) ToRawTextOnly() (s []string) {
	for _, metar := range r.Data.Pireps {
		s = append(s, metar.RawText)
	}
	return
}

func (r *Response) ToJson() (s string, err error) {
	bytes, err := json.Marshal(r.Data.Pireps)
	if err != nil {
		return "", err
	}
	s = string(bytes)
	return
}

func (r *Response) ToJsonIndented() (s string, err error) {
	bytes, err := json.MarshalIndent(r.Data.Pireps, "", "  ")
	if err != nil {
		return "", err
	}
	s = string(bytes)
	return
}

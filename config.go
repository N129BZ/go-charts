package main

import (
	"encoding/json"
	"os"
	"log"
)

// Configuration contains all of the configuration values
type Configuration struct {
	Savepositionhistory   bool   `json:"savepositionhistory"`
	Histintervalmsec      int    `json:"histintervalmsec"`
	Getgpsfromstratux     bool   `json:"getgpsfromstratux"`
	Gpsintervalmsec       int    `json:"gpsintervalmsec"`
	Wxupdateintervalmsec  int    `json:"wxupdateintervalmsec"`
	Keepaliveintervalmsec int    `json:"keepaliveintervalmsec"`
	Httpport              int    `json:"httpport"`
	Startupzoom           int    `json:"startupzoom"`
	Debug                 bool   `json:"debug"`
	HistoryDb             string `json:"historyDb"`
	Uselocaltime          bool   `json:"uselocaltime"`
	Distanceunit          string `json:"distanceunit"`
	Stratuxurl            string `json:"stratuxurl"`
	Animatedwxurl         string `json:"animatedwxurl"`
	MetarsURL             string `json:"metarsurl"`
	TafsURL               string `json:"tafsurl"`
	PirepsURL             string `json:"pirepsurl"`
	Lockownshiptocenter   bool   `json:"lockownshiptocenter"`
	Ownshipimage          string `json:"ownshipimage"`
	Usemetricunits        bool   `json:"usemetricunits"`
	Distanceunits         struct {
		Kilometers    string `json:"kilometers"`
		Nauticalmiles string `json:"nauticalmiles"`
		Statutemiles  string `json:"statutemiles"`
	} `json:"distanceunits"`
	Messagetypes struct {
		Metars struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		} `json:"metars"`
		Tafs struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		} `json:"tafs"`
		Pireps struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		} `json:"pireps"`
		Airports struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		} `json:"airports"`
	} `json:"messagetypes"`
}

var config Configuration

// GetConfigAsString returns configuration data as a json string for client use
func GetConfigAsString() (string, error) {
	LoadConfig()
	data, err := json.Marshal(&config)
	if err != nil {
		return "", err
	}
	sdata := string(data)
	return sdata, nil
}

// LoadConfig loads a Config struct from a json file for server use
func LoadConfig() (error) {
	data, err := os.ReadFile("./config.json")
	if err != nil {
		log.Println(err)
		return err
	}
	json.Unmarshal(data, &config)
	return nil
}
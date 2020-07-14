package cmd

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

type spectra struct {
	filename     string
	data         [][]string
	N            int
	nuMax, nuMin float64
	conditions   []string
}

const defaultZipName = "SpectraPlotSimulations.zip"

type byNuMin []spectra

func (a byNuMin) Len() int           { return len(a) }
func (a byNuMin) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byNuMin) Less(i, j int) bool { return a[i].nuMin < a[j].nuMin }

func processSpectra(zipName, outputDir string) error {
	_, err := os.Stat(outputDir)
	if os.IsNotExist(err) {
		return err
	}
	r, err := zip.OpenReader(zipName)
	if err != nil {
		return err
	}
	defer r.Close()
	var allRecords []spectra
	var conditions []string
	for _, f := range r.File {
		rc, err := f.Open()
		defer rc.Close()
		records, err := csv.NewReader(rc).ReadAll()
		if err != nil {
			return err
		}
		wavenumMax, err := strconv.ParseFloat(records[len(records)-1][0], 64)
		if err != nil {
			return err
		}
		wavenumMin, err := strconv.ParseFloat(records[1][0], 64)
		if err != nil {
			return err
		}
		c := strings.Split(records[0][1], "/")
		if conditions == nil {
			conditions = c
		}
		for i, v := range conditions {
			if c[i] != v {
				return fmt.Errorf("gas absorption conditions differ")
			}
		}
		allRecords = append(allRecords, spectra{
			filename:   f.Name,
			data:       records,
			N:          len(records),
			nuMax:      wavenumMax,
			nuMin:      wavenumMin,
			conditions: c,
		})
	}
	if len(allRecords) == 0 {
		return fmt.Errorf("no files processed in zip")
	}
	sort.Sort(byNuMin(allRecords))
	minWN, maxWN := allRecords[0].nuMin, allRecords[len(allRecords)-1].nuMax
	spectraCond, err := parseSpectraConditions(conditions)
	if err != nil {
		return err
	}
	outputName := generateFilename(spectraCond, [2]float64{minWN, maxWN}) // fmt.Sprintf("nu=%.f-%.f%s%s.csv", minWN, maxWN, sep, strings.Join(conditions, sep))
	f, err := os.Create(outputDir + fpsep + outputName)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	err = w.Write(generateHeader(conditions))
	if err != nil {
		return err
	}
	for _, v := range allRecords {
		err = w.WriteAll(v.data[1:])
		if err != nil {
			return err
		}
	}
	return nil
}

func generateHeader(conditions []string) (h []string) {
	h = append(h, "nu")
	cond := strings.Join(conditions, "/")
	return append(h, cond)
}

func parseSpectraConditions(conditionSlice []string) (c spectraConditions, err error) {
	var f float64
	for _, val := range conditionSlice {
		keyval := strings.Split(val, "=")
		if len(keyval) > 2 {
			err = fmt.Errorf("expected spectra key-value in parseSpectraConditions")
			return
		} else if len(keyval) == 1 {
			c.gasID = keyval[0]
			continue
		}
		switch keyval[0] {
		case "x":
			f, err = strconv.ParseFloat(keyval[1], 64)
			c.Ppm = f * 1e6
		case "T":
			f, err = strconv.ParseFloat(strings.ReplaceAll(keyval[1], "K", ""), 64)
			c.T = f
		case "P":
			f, err = strconv.ParseFloat(strings.ReplaceAll(keyval[1], "atm", ""), 64)
			c.P = f
		case "L":
			f, err = strconv.ParseFloat(strings.ReplaceAll(keyval[1], "cm", ""), 64)
			c.L = f
		default:
			err = fmt.Errorf("unknown key value pair %s:%s", keyval[0], keyval[1])
		}
		if err != nil {
			break
		}
	}
	return
}

func generateFilename(c spectraConditions, interval [2]float64) string {
	var strcond []string
	sep := ","
	strcond = append(strcond, c.gasID,
		"x="+prettyF(c.Ppm*1e-6), "T="+prettyF(c.T)+"K", "P="+prettyF(c.P)+"atm", "L="+prettyF(c.L)+"cm")
	return fmt.Sprintf("nu=%.f-%.f%s%s.csv", interval[0], interval[1], sep, strings.Join(strcond, sep))
}

func prettyF(f float64) string {
	format := `%{front}.{back}`
	isNegative := f < 0
	f = math.Abs(f)
	if f == 0 {
		return "0"
	}
	if f+0.001 > 1e3 {
		f = 0.001 + f
	} else if f+1e-7 > 1e-3 {
		f = f + 1e-7
	}
	if f >= 1e3 {
		f = math.Floor(f)
	}
	dec := f - math.Floor(f)
	if dec > 0 && dec >= 1e-3  {
		format = strings.Replace(format, "{back}", "3", 1)
	}
	if f <= 1e-3 || f >= 1e3 {
		format = format + "e"
		form:=fmt.Sprintf("%.3e",f)
		if strings.Count(form,"0") < 3 {
			format = strings.Replace(format, "{back}", "3", 1)
		}
	} else {
		format = format + "f"
	}
	if isNegative {
		f = -1 * f
	}
	format = strings.Replace(format, "{front}", "", 1)
	format = strings.Replace(format, "{back}", "", 1)
	return fmt.Sprintf(format, f)
}

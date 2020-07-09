package cmd

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
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
	sep := ","
	outputName := fmt.Sprintf("nu=%.f-%.f%s%s.csv", minWN, maxWN, sep, strings.Join(conditions, sep))
	f, err := os.Create(outputDir + string(filepath.Separator) + outputName)
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

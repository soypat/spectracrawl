/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	wd "github.com/fedesog/webdriver"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var logFile *os.File

type spectraConditions struct {
	T, P, L, NuStart, NuEnd, NuStep, Ppm float64
	gasID                                string
}

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "spectracrawl",
	Short: "Scrapes spectraplot gas a absorption data",
	Long: `Scrapes spectraplot gas a absorption data

See https://www.cfa.harvard.edu/hitran/Download/HITRAN2012.pdf
for a complete paper on HITRAN2012 spectral coverage of 
available gases.

Code is open source and protected by Apache license 2.0

Code and example config file at:
http://github.com/soypat/spectracrawl
`,
	Args: func(cmd *cobra.Command, args []string) error {
		err := checkConfig()
		if err != nil {
			logf("[err] error in config. %s", err)
		}
		return err
	},
	Run: func(cmd *cobra.Command, args []string) {
		log("[inf] starting program")
		if err := runner(args); err != nil {
			logf("[err] %s",err)
			time.Sleep(time.Second*10)
			os.Exit(1)
		}
	},
}

const fpsep = string(filepath.Separator)

const (
	maxWaveNumber        = 47365.0
	maxTemp              = 4e12
	minNuStep            = 0.01
	downloadTimeoutAfter = time.Second * 2
)
const urlStart = "http://www.spectraplot.com/absorption"

func runner(_ []string) error {
	chromeDriver := wd.NewChromeDriver(viper.GetString("browser.driverPath"))
	downloadPath := viper.GetString("browser.downloadDir")
	downloadedFileName := downloadPath + fpsep + defaultZipName
	err := chromeDriver.Start()
	if err != nil {
		return err
	}
	var session *wd.Session
	desired := wd.Capabilities{"Platform": "Windows"}
	required := wd.Capabilities{"Platform": "Windows"}
	session, err = chromeDriver.NewSession(desired, required)
	if err != nil {
		return err
	}
	defer session.CloseCurrentWindow()
	defer session.Delete()
	if err = session.Url(urlStart); err != nil {
		return err
	}
	startNu, endNu := viper.GetFloat64("HITRAN.startNu"), viper.GetFloat64("HITRAN.endNu")
	intervals := nuIntervals(startNu, endNu)
	plotCount := 0
	_ = os.Remove(downloadedFileName) // delete any previous spectraplot file if present
	for intervalCount, interval := range intervals {
		err = setHitran(session, spectraConditions{
			T:       viper.GetFloat64("HITRAN.T"),
			P:       viper.GetFloat64("HITRAN.p"),
			L:       viper.GetFloat64("HITRAN.L"),
			NuStart: interval[0],
			NuEnd:   interval[1],
			NuStep:  viper.GetFloat64("HITRAN.stepNu"),
			Ppm:     viper.GetFloat64("HITRAN.ppm"),
			gasID:   viper.GetString("HITRAN.gasID"),
		})
		if err != nil {
			return err
		}
		err = waitForCalculation(session)
		if err != nil {
			_ = leftClickSelector(session, `#clear`)
			continue
		}
		time.Sleep(time.Duration(viper.GetInt("spectraplot.calcDelay_s"))*time.Second)
		logf("[scp] calculating nu=[%.f-%.f] for %s",interval[0],interval[1],viper.GetString("HITRAN.gasID"))
		_ = leftClickSelector(session, `#calculate_hitran`)
		plotCount++
		if plotCount == viper.GetInt("spectraplot.maxNumberOfPlots") || intervalCount == len(intervals)-1 {
			waitForCalculation(session)
			logf("[scp] downloading file. finished %d/%d",intervalCount+1,len(intervals))
			_ = leftClickSelector(session, `#data`)
			_ = leftClickSelector(session, `#clear`)
			err = waitForDownload(downloadedFileName)
			err = processSpectra(downloadedFileName, viper.GetString("output.dir"))
			if err != nil {
				return err
			}
			err = os.Remove(downloadedFileName)
			if err != nil {
				return err
			}
			plotCount = 0
		}
	}
	log("[inf] finish program")
	return nil
}

func waitForDownload(downloadName string) error {
	downloaded := false
	timeout := false
	var err error
	go func() {
		time.Sleep(time.Duration(viper.GetInt("output.timeout_s")) * time.Second)
		timeout = true
	}()
	for !downloaded {
		_, err = os.Stat(downloadName)
		if os.IsNotExist(err) {
			time.Sleep(100 * time.Millisecond)
		} else {
			downloaded = true
		}
		if timeout {
			return fmt.Errorf("download wait timed out")
		}
	}
	return err
}

func checkConfig() error {
	if viper.GetBool("log.toFile") {
		fo, err := os.Create("sgacrawl.log")
		if err != nil {
			return err
		}
		logFile = fo
		defer logFile.Close()
	}
	log("[inf] start program")
	// OVERRIDES
	if gasFlag != "" {
		viper.Set("HITRAN.gasID", gasFlag)
	}
	if nuSFlag >= 0 {
		viper.Set("HITRAN.startNu",nuSFlag)
	}
	if nuEFlag >= 0 {
		viper.Set("HITRAN.endNu",nuEFlag)
	}
	if ppmFlag >= 0 {
		viper.Set("HITRAN.ppm",ppmFlag)
	}
	// Config file information sanitizing
	// timeouts and delays
	downloadTimeout := viper.GetInt("output.timeout_s")
	if downloadTimeout <= 0 {
		log("[inf] download timeout (output.timeout_s) set to 99 seconds")
		viper.Set("output.timeout_s", 99)
	}
	calcTimeout := viper.GetInt("spectraplot.calcTimeout_s")
	if calcTimeout <= 0 {
		log("[inf] spectraplot.calcTimeout_s set to 99 seconds")
		viper.Set("spectraplot.calcTimeout_s", 99)
	}
	calcDelay:=viper.GetInt("spectraplot.calcDelay_s")
	if calcDelay < 0 {
		log("[inf] calc delay (spectraplot.calcDelay_s) set to 1 second")
		viper.Set("spectraplot.calcDelay_s", 1)
	}
	// HITRAN
	nuStart, nuEnd := viper.GetFloat64("HITRAN.startNu"), viper.GetFloat64("HITRAN.endNu")
	lambdaStart, lambdaEnd := viper.GetFloat64("HITRAN.startLambda"), viper.GetFloat64("HITRAN.endLambda")
	if nuStart == 0 && nuEnd == 0 {
		nuStart, nuEnd = waveLtoNum(lambdaStart), waveLtoNum(lambdaEnd)
		viper.Set("HITRAN.startNu", nuStart)
		viper.Set("HITRAN.endNu", nuEnd)
	}
	if nuStart < 0 || nuStart > maxWaveNumber || nuEnd < 0 || nuEnd > maxWaveNumber {
		return fmt.Errorf("exceeded spectral range [0-%f]. got vs=%f, ve=%f", maxWaveNumber, nuStart, nuEnd)
	}
	logf("[inf] scraping wavenumbers:[%.f-%.f]", nuStart, nuEnd)
	if stepNu := viper.GetFloat64("HITRAN.stepNu"); stepNu < minNuStep {
		viper.Set("HITRAN.stepNu", minNuStep)
		logf("[inf] HITRAN.stepNu too low or not present. setting at %.2f", minNuStep)
	}
	T, p, L := viper.GetFloat64("HITRAN.T"), viper.GetFloat64("HITRAN.p"), viper.GetFloat64("HITRAN.L")
	if T <= 0 || T > maxTemp || p <= 0 || L <= 0 {
		return fmt.Errorf("temp to high or negative/zero value for pressure/temp/length.")
	}

	if ppm := viper.GetFloat64("HITRAN.ppm"); ppm <= 0 || ppm > 1e6 {
		return fmt.Errorf("ppm <= 0 or greater than 1e6. got ppm = %f", ppm)
	}
	format := viper.GetString("HITRAN.format")
	if _, err := strconv.ParseFloat(fmt.Sprintf(format, T), 64); err != nil {
		return fmt.Errorf("formatter '%s' invalid for float. %s", format, err.Error())
	}
	// paths and files
	// first sanitize paths
	viper.Set("browser.downloadDir", sanitizePath(viper.GetString("browser.downloadDir")))
	viper.Set("browser.driverPath", sanitizePath(viper.GetString("browser.driverPath")))
	viper.Set("output.dir", sanitizePath(viper.GetString("output.dir")))
	downloadDir := viper.GetString("browser.downloadDir")
	_, err := os.Stat(downloadDir)
	if os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist. %s", err)
	}
	driverPath := viper.GetString("browser.driverPath")
	_, err = os.Stat(driverPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("driver does not exist in path given. %s", err)
	}
	gasID := viper.GetString("HITRAN.gasID")
	if gasID == ""  {
		return fmt.Errorf("null HITRAN.gasID")
	}
	outputPath := viper.GetString("output.dir")
	if outputPath == "auto" {
		outputPath = fmt.Sprintf("."+fpsep+"output"+fpsep+"%s", gasID)
	}
	_, err = os.Stat(outputPath)
	if os.IsNotExist(err) {
		logf("[inf] creating output directory %s", outputPath)
		err = os.MkdirAll(outputPath, os.ModePerm)
		if err != nil {
			return err
		}
	}
	viper.Set("browser.downloadDir", downloadDir)
	viper.Set("output.dir", outputPath)
	return nil
}

func setHitran(s *wd.Session, conditions spectraConditions) error {
	Telem, err := query(s, `#hitran > div > div > table > tbody > tr:nth-child(1) > td:nth-child(2) > input[type=text]`)
	if err != nil {
		return err
	}
	Pelem, err := query(s, `#hitran > div > div > table > tbody > tr:nth-child(2) > td:nth-child(2) > input[type=text]`)
	if err != nil {
		return err
	}
	Lelem, err := query(s, `#hitran > div > div > table > tbody > tr:nth-child(3) > td:nth-child(2) > input[type=text]`)
	if err != nil {
		return err
	}
	nuStartelem, err := query(s, `#hitran > div > div > table > tbody > tr:nth-child(1) > td:nth-child(5) > input[type=text]`)
	if err != nil {
		return err
	}
	nuEndelem, err := query(s, `#hitran > div > div > table > tbody > tr:nth-child(2) > td:nth-child(5) > input[type=text]`)
	if err != nil {
		return err
	}
	nuStepelem, err := query(s, `#hitran > div > div > table > tbody > tr:nth-child(3) > td:nth-child(5) > input[type=text]`)
	if err != nil {
		return err
	}
	//lamStartelem, err := query(s, `#hitran > div > div > table > tbody > tr:nth-child(1) > td:nth-child(3) > input[type=text]`)
	//if err != nil {
	//	return err
	//}
	//lamEndelem, err := query(s, `#hitran > div > div > table > tbody > tr:nth-child(2) > td:nth-child(3) > input[type=text]`)
	//if err != nil {
	//	return err
	//}
	Ppmelem, err := query(s, `#hitran > div > div > table > tbody > tr:nth-child(1) > td:nth-child(7) > input[type=text]`)
	if err != nil {
		return err
	}
	var format string
	if format = viper.GetString("HITRAN.format"); format == "" {
		format = "%.3f"
	}
	Telem.Clear()
	err = Telem.SendKeys(fmt.Sprintf(format, conditions.T))
	if err != nil {
		return err
	}
	Pelem.Clear()
	Pelem.SendKeys(fmt.Sprintf(format, conditions.P))
	Lelem.Clear()
	Lelem.SendKeys(fmt.Sprintf(format, conditions.L))
	nuEndelem.Clear()
	nuEndelem.SendKeys(fmt.Sprintf(format, conditions.NuEnd))
	nuStartelem.Clear()
	nuStartelem.SendKeys(fmt.Sprintf(format, conditions.NuStart))
	nuStepelem.Clear()
	nuStepelem.SendKeys(fmt.Sprintf("%0.3f", conditions.NuStep))
	Ppmelem.Clear()
	Ppmelem.SendKeys(fmt.Sprintf(strings.Replace(format, "f", "e", 1), conditions.Ppm*1e-6))
	gasButton, _ := s.FindElement("xpath", `//*[@id="multicol-menu"]`)
	gasButton.Click()
	gasColumnElem, err := s.FindElements("xpath", `//*[@id="multicol-menu"]/li/ul/li/div[1]/ul`)
	if err != nil {
		return err
	}
	for _, v := range gasColumnElem {
		gasElem, err := v.FindElements("xpath", `li/a`)
		if err != nil {
			return err
		}
		for _, e := range gasElem {
			gasName, _ := e.Text()
			if gasName == viper.GetString("HITRAN.gasID") {
				e.Click()
			}
		}
	}
	return nil
}

func waveLtoNum(λ float64) float64  { return 1e4 / λ }
func waveNumtoL(nu float64) float64 { return 1e4 / nu }

func waitForCalculation(s *wd.Session) error {
	loaded := false
	timeout := false
	go func() {
		time.Sleep(time.Duration(viper.GetInt("spectraplot.calcTimeout_s")) * time.Second)
		timeout = true
	}()
	submitButton, _ := s.FindElement("css selector", `#calculate_hitran`)
	for !loaded {
		text, _ := submitButton.Text()
		if text != "Calculating..." {
			loaded = true
		} else {
			time.Sleep(time.Millisecond * 100)
		}
		if timeout {
			return fmt.Errorf("timeout")
		}
	}
	return nil
}

func nuIntervals(nuStart, nuEnd float64) (intervals [][2]float64) {
	maxRange := viper.GetFloat64("spectraplot.maxRange")
	if nuStart > nuEnd {
		nuStart, nuEnd = nuEnd, nuStart
	}
	for start := nuStart; start < nuEnd-1; start += maxRange {
		end := start + maxRange
		if start+maxRange > nuEnd {
			end = nuEnd
		}
		intervals = append(intervals, [2]float64{start, end})
	}
	return intervals
}

func sanitizePath(path string) string {
	fpsep := string(filepath.Separator)
	path = strings.ReplaceAll(strings.ReplaceAll(path, "\\", fpsep), "/", fpsep)
	path = strings.TrimSuffix(path, fpsep)
	return path
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var gasFlag string
var ppmFlag, nuSFlag, nuEFlag float64
func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", ".spectracrawl.yml", "config file (default is $HOME/.spectracrawl.yaml)")
	rootCmd.PersistentFlags().StringVar(&gasFlag, "gas", "", "HITRAN.gasID override")
	rootCmd.PersistentFlags().Float64Var(&ppmFlag, "ppm",-1,"HITRAN.ppm override")
	rootCmd.PersistentFlags().Float64Var(&nuSFlag, "nu1",-1,"HITRAN.startNu override")
	rootCmd.PersistentFlags().Float64Var(&nuEFlag, "nu2",-1,"HITRAN.endNu override")

}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".spectracrawl" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".spectracrawl")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

func log(args ...interface{}) {
	logf("%s", args...)
}

func logf(format string, args ...interface{}) {
	var msg string
	if len(args) == 0 {
		msg = fmt.Sprintf(format)
	} else {
		msg = fmt.Sprintf(format, args...)
	}
	msg = strings.TrimSuffix(msg, "\n") + "\n"
	if !viper.GetBool("log.silent") {
		fmt.Print(msg)
	}
	if viper.GetBool("log.toFile") {
		_, _ = logFile.WriteString(msg)
		_ = logFile.Sync()
	}
}

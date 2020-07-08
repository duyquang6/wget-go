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
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var cfgFile string
var filepath string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "wget-go",
	Short: "Download single file, support resume download",
	//	Long: `A longer description that spans multiple lines and likely contains
	//examples and usage of using your application. For example:
	//
	//Cobra is a CLI library for Go that empowers applications.
	//This application is a tool to generate the needed files
	//to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		sigs := make(chan os.Signal, 1)
		done := make(chan bool, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

		fileUrl := "https://mirrors.bkns.vn/ubuntu-releases/20.04/ubuntu-20.04-desktop-amd64.iso"
		//fileUrl := "https://ftp.gnu.org/gnu/wget/wget-1.20.tar.gz"

		go func() {
			err := DownloadFile(filepath, fileUrl)
			if err != nil {
				log.Println(err)
				done <- false
			}
			done <- true
		}()

		select {
		case <-done:
			fmt.Println("----------DONE-----------")
		case <-sigs:
			log.Println("\nStop forcely, remove temp file")
			if err := os.Remove(filepath + ".tmp"); err != nil {
				log.Println(err)
			}
			log.Println("Remove", filepath+".tmp", "successfully")
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}


// WriteCounter counts the number of bytes written to it. It implements to the io.Writer interface
// and we can pass this into io.TeeReader() which will report progress on each write cycle.
type WriteCounter struct {
	Meta    FileMetadata
	Percent float64
	Total   uint64

	_startTime time.Time
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Total += uint64(n)
	wc.PrintProgress()
	return n, nil
}

func (wc WriteCounter) PrintProgress() {
	// Clear the line by using a character return to go back to the start and remove
	// the remaining characters by filling it with spaces
	fmt.Printf("\r%s", strings.Repeat(" ", 100))

	perc := float64(wc.Total) / float64(wc.Meta.FileSize) * 100
	now := time.Now()
	totTime := now.Sub(wc._startTime)
	spd := float64(wc.Total/1000) / totTime.Seconds()
	eta := float64(wc.Meta.FileSize-wc.Total) / 1000.0 / spd
	fmt.Printf("\rDownloading... %s complete, percent: %.1f%%, speed: %.1fM, eta: %s",
		humanize.Bytes(wc.Total), perc, spd, humanize.Time(now.Add(time.Duration(int64(eta*float64(time.Second))))))
}

type FileMetadata struct {
	ContentType string
	FileSize    uint64
}

func DownloadFile(filepath string, _url string) error {
	u, err := url.Parse(_url)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Resolving %s ...", u.Host)

	ips, err := net.LookupIP(u.Host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not get IPs: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(ips[0].String())

	out, err := os.Create(filepath + ".tmp")
	if err != nil {
		return err
	}

	fmt.Print("HTTP request sent, awaiting response...")
	resp, err := http.Get(_url)
	fmt.Println(resp.Status)
	if err != nil {
		out.Close()
		return err
	}
	defer resp.Body.Close()

	meta := FileMetadata{
		ContentType: resp.Header.Get("Content-Type"),
		FileSize:    uint64(resp.ContentLength),
	}

	fmt.Println("----------METADATA-----------")
	fmt.Println("File size:", humanize.Bytes(meta.FileSize))
	fmt.Println("File type:", meta.ContentType)

	fmt.Println("----------PROCESSING-----------")

	// Create our progress reporter and pass it to be used alongside our writer
	counter := &WriteCounter{Meta: meta, _startTime: time.Now()}
	if _, err = io.Copy(out, io.TeeReader(resp.Body, counter)); err != nil {
		out.Close()
		return err
	}

	// The progress use the same line so print a new line once it's finished downloading
	fmt.Print("\n")

	// Close the file without defer so it can happen before Rename()
	out.Close()

	if err = os.Rename(filepath+".tmp", filepath); err != nil {
		return err
	}
	return nil
}



func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.wget-go.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	filepath = *rootCmd.Flags().StringP("output","o" ,"testing", "Output location path")
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

		// Search config in home directory with name ".wget-go" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".wget-go")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Result struct {
	Host      string   `json:"host"`
	Reachable bool     `json:"reachable"`
	OpenPorts []string `json:"open_ports,omitempty"`
}

const (
	subdomainFile = "subdomains.dat"
	resultFile    = "subdomain-results.json"
	maxWorkers    = 50
	timeout       = 2 * time.Second
)

var totalScanned uint32

func main() {
	fmt.Print("\033[1;32mEnter domain (e.g., google.com):\033[0m ")
	var domain string
	fmt.Scanln(&domain)

	fmt.Print("\033[1;32mEnter ports to scan (comma-separated, optional):\033[0m ")
	var portInput string
	fmt.Scanln(&portInput)
	ports := parsePorts(portInput)

	subdomains, err := readSubdomains(subdomainFile)
	if err != nil {
		fmt.Printf("\033[1;31mError reading %s: %v\033[0m\n", subdomainFile, err)
		return
	}

	jobs := make(chan string, len(subdomains))
	results := make(chan Result, len(subdomains))
	var wg sync.WaitGroup

	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go worker(domain, ports, jobs, results, &wg, len(subdomains))
	}

	for _, sub := range subdomains {
		jobs <- sub
	}
	close(jobs)

	wg.Wait()
	close(results)

	var allResults []Result
	for res := range results {
		if res.Reachable {
			allResults = append(allResults, res)
		}
	}

	saveResults(allResults)
	fmt.Printf("\n\033[1;34mDone. Results saved to %s\033[0m\n", resultFile)

	if len(allResults) > 0 {
		fmt.Println("\n\033[1;36mReachable Subdomains:\033[0m")
		for _, res := range allResults {
			fmt.Printf("  \033[1;32m%-30s\033[0m", res.Host)
			if len(res.OpenPorts) > 0 {
				var portDisplay []string
				for _, p := range res.OpenPorts {
					portDisplay = append(portDisplay, fmt.Sprintf("\033[1;35m%s\033[0m", p))
				}
				fmt.Printf(" Open ports: %s", strings.Join(portDisplay, ", "))
			}
			fmt.Println()
		}
	} else {
		fmt.Println("\n\033[1;31mNo subdomains found reachable.\033[0m")
	}
}

func worker(domain string, ports []string, jobs <-chan string, results chan<- Result, wg *sync.WaitGroup, total int) {
	defer wg.Done()
	for sub := range jobs {
		fullHost := fmt.Sprintf("%s.%s", sub, domain)
		ipAddrs, err := net.LookupHost(fullHost)
		if err != nil || len(ipAddrs) == 0 {
			updateProgress(total)
			continue
		}

		result := Result{Host: fullHost, Reachable: true}

		if len(ports) > 0 {
			for _, port := range ports {
				if isPortOpen(fullHost, port) {
					result.OpenPorts = append(result.OpenPorts, port)
				}
			}
		}

		results <- result
		updateProgress(total)
	}
}

func updateProgress(total int) {
	current := atomic.AddUint32(&totalScanned, 1)
	fmt.Printf("\r\033[1;33mScanned %d / %d subdomains\033[0m", current, total)
}

func isPortOpen(host, port string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func parsePorts(input string) []string {
	if input == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	var ports []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			ports = append(ports, trimmed)
		}
	}
	return ports
}

func readSubdomains(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var subs []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			subs = append(subs, line)
		}
	}
	return subs, scanner.Err()
}

func saveResults(results []Result) {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		fmt.Printf("\033[1;31mFailed to write results: %v\033[0m\n", err)
		return
	}
	_ = os.WriteFile(resultFile, data, 0644)
}

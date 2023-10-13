package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
)

const (
	msgContainerKilled = "Pod is marked for graceful deletion, begin teardown"
	plegRelist         = "GenericPLEG: Relisting"
)

type message struct {
	Timestamp float64 `json:"ts"`
	Message   string  `json:"msg"`
	Pod       struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"pod"`
	Pods   []string `json:"pods"`
	Caller string   `json:"caller"`
}

func main() {
	var (
		pod               string
		stopAfterDeletion bool
	)
	flag.StringVar(&pod, "pod", "", "the pod to analyze the logs for")
	flag.BoolVar(&stopAfterDeletion, "stop-after-deletion", false, "stop log analyzing after seeing a deletion")
	flag.Parse()

	if pod == "" {
		log.Fatalln("No pod provided")
	}
	fmt.Println("Pod: " + pod)

	fmt.Println("ELAPSED\tDIFF\tSYSTEM\tMESSAGE")
	start := 0.
	last := 0.
	consumeMessage := func(msg message) {
		if start == 0 {
			start = msg.Timestamp
		}

		// Figure out which subsystem the line belongs to.
		subsystem := color.New(color.Bold).SprintFunc()("MISC")
		if strings.HasPrefix(msg.Caller, "volumemanager/") || strings.HasPrefix(msg.Caller, "populator/") || strings.HasPrefix(msg.Caller, "reconciler/") || strings.HasPrefix(msg.Caller, "operationexecutor/") {
			subsystem = color.New(color.Bold, color.FgGreen).SprintFunc()("VOLUME")
		} else if strings.HasPrefix(msg.Caller, "kuberuntime/") || msg.Message == "syncPod enter" || msg.Message == "syncPod exit" {
			subsystem = color.New(color.Bold, color.FgBlue).SprintFunc()("SYNCPOD")
		} else if strings.HasPrefix(msg.Caller, "pleg/") {
			subsystem = color.New(color.Bold, color.FgRed).SprintFunc()("PLEG")
		} else if strings.HasPrefix(msg.Caller, "status/") {
			subsystem = color.New(color.Bold, color.FgHiBlue).SprintFunc()("STATUS")
		} else if strings.HasPrefix(msg.Caller, "kubelet/kubelet_pods") {
			subsystem = color.New(color.Bold, color.FgHiGreen).SprintFunc()("MOUNT")
		} else if strings.HasPrefix(msg.Caller, "prober") {
			subsystem = color.New(color.Bold, color.FgYellow).SprintFunc()("PROBE")
		}

		diff := dur(msg.Timestamp - last)
		dv := int(msg.Timestamp - last)
		last = msg.Timestamp

		diffStr := fmt.Sprintf("%-9s", diff)
		if dv > 10 {
			diffStr = color.New(color.FgYellow).Sprintf("%-9s", diff)
		}
		if dv > 30 {
			diffStr = color.New(color.Bold, color.FgYellow).Sprintf("%-9s", diff)
		}
		if dv > 50 {
			diffStr = color.New(color.FgHiYellow).Sprintf("%-9s", diff)
		}
		if dv > 100 {
			diffStr = color.New(color.Bold, color.FgHiYellow).Sprintf("%-9s", diff)
		}
		if dv > 300 {
			diffStr = color.New(color.FgRed).Sprintf("%-9s", diff)
		}
		if dv > 500 {
			diffStr = color.New(color.Bold, color.FgRed).Sprintf("%-9s", diff)
		}

		fmt.Printf("%-9s\t%s\t%s\t%s\n", dur(msg.Timestamp-start), diffStr, subsystem, truncate(msg.Message, 90))
	}

	var seenFirstPodMessage bool
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		start := strings.Index(line, "{")
		if start == -1 {
			continue
		}
		line = line[start:]

		var msg message
		json.Unmarshal([]byte(line), &msg)

		if seenFirstPodMessage && msg.Message == plegRelist {
			consumeMessage(msg)
			continue
		}

		if strings.HasPrefix(msg.Pod.Name, pod) {
			if stopAfterDeletion && msg.Message == msgContainerKilled {
				break
			}

			seenFirstPodMessage = true
			consumeMessage(msg)
			continue
		}

		for _, podName := range msg.Pods {
			if podName == pod {
				seenFirstPodMessage = true
				consumeMessage(msg)
				break
			}
		}
	}
}

func dur(f float64) time.Duration {
	return time.Duration(float64(time.Millisecond) * f).Round(time.Microsecond * 100)
}

func truncate(msg string, max int) string {
	if len(msg) < max {
		return msg
	}
	return msg[:max-3] + "..."
}

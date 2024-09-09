package command

import (
	"context"
	"fmt"
	"gosshpuppet/internal/callback"
	"gosshpuppet/internal/config"
	"gosshpuppet/internal/puppet"
	"io"
	"slices"
	"strings"
)

func AdminInterpreter(pm *puppet.Manager, ac *config.AccessConfigHolder) callback.CommandInterpreter {
	return func(ctx context.Context, user string, args []string, w io.Writer) error {
		if len(args) == 0 {
			fmt.Fprintln(w, "Available commands: ls")
			return nil
		}

		switch args[0] {
		case "ls":
			adminPrintPuppets(ctx, w, pm, ac)
		default:
			fmt.Fprintln(w, "Unknown command")
		}

		return nil
	}
}

func adminPrintPuppets(ctx context.Context, w io.Writer, pm *puppet.Manager, ac *config.AccessConfigHolder) {
	accessConfig := ac.Load()

	pp := pm.Puppets()

	if len(pp) == 0 {
		fmt.Fprintln(w, "No puppets")
		return
	}

	puppetNames := make([]string, 0, len(pp))
	for name := range pp {
		puppetNames = append(puppetNames, name)
	}
	slices.Sort(puppetNames)

	ports := make([]uint32, 0)
	namedPort := make([]string, 0)

	tableHeader := []string{"PUPPET", "PORTS"}
	tableRows := make([][]string, 0, len(pp))

	for _, puppetName := range puppetNames {
		ports = ports[:0]
		namedPort = namedPort[:0]

		for k := range pp[puppetName] {
			ports = append(ports, k)
		}
		slices.Sort(ports)

		for _, v := range ports {
			name, ok := accessConfig.Services[v]
			if !ok {
				name = "unknown"
			}
			namedPort = append(namedPort, fmt.Sprintf("%s=%d", name, v))
		}

		tableRows = append(tableRows, []string{puppetName, strings.Join(namedPort, ",")})

		select {
		case <-ctx.Done():
			return
		default:
		}
	}

	printTable(w, "  ", tableHeader, tableRows)
}

func printTable(w io.Writer, indent string, headers []string, rows [][]string) {
	// Calculate column widths
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print headers
	for i, header := range headers {
		fmt.Fprintf(w, "%-*s", widths[i], header)
		fmt.Fprint(w, indent)
	}
	fmt.Fprintln(w)

	// Print rows
	for _, row := range rows {
		for i, cell := range row {
			fmt.Fprintf(w, "%-*s", widths[i], cell)
			fmt.Fprint(w, indent)
		}
		fmt.Fprintln(w)
	}
}

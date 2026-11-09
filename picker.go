package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// pickerColumns is how many regions are shown per row in the picker.
const pickerColumns = 3

// pickerChoices lists the regions offered by the picker: "local" first, then
// every preset in alphabetical order. Numbers in the picker start at 1.
func pickerChoices() []string {
	return append([]string{"local"}, PresetNames()...)
}

// pickRegions shows every region as a numbered list and reads the user's
// choice from in. It returns a -regions value. Pressing Enter keeps the defaults.
func pickRegions(in io.Reader, out io.Writer, o options) (string, error) {
	choices := pickerChoices()
	isDefault := map[string]bool{}
	for _, d := range DefaultRegions {
		isDefault[d] = true
	}

	fmt.Fprintln(out, "Which regions do you want to test? (* = default)")
	fmt.Fprintln(out)
	rows := (len(choices) + pickerColumns - 1) / pickerColumns
	for r := 0; r < rows; r++ {
		var line strings.Builder
		for c := 0; c < pickerColumns; c++ {
			i := c*rows + r // fill columns top to bottom
			if i >= len(choices) {
				break
			}
			mark := " "
			if isDefault[choices[i]] {
				mark = errColors.accent("*")
			}
			fmt.Fprintf(&line, "  %s%s %-16s", errColors.dim(fmt.Sprintf("%2d", i+1)), mark, displayName(choices[i]))
		}
		fmt.Fprintln(out, strings.TrimRight(line.String(), " "))
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, `Type numbers or names (e.g. "1 7 18" or "local,tokyo"), "all", or press Enter for *.`)

	sc := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "> ")
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				return "", err
			}
			return "", errors.New("no regions chosen")
		}
		picked, err := parsePick(sc.Text(), choices)
		if err != nil {
			fmt.Fprintf(out, "%v. Try again.\n", err)
			continue
		}
		n := len(picked) * o.k
		fmt.Fprintf(out, "%d region(s), %d servers, about %s.\n\n",
			len(picked), n, fmtDur(estimate(n, o)))
		return strings.Join(picked, ","), nil
	}
}

// parsePick turns the user's answer into region keys. An empty answer means
// the defaults.
func parsePick(answer string, choices []string) ([]string, error) {
	fields := strings.FieldsFunc(answer, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	if len(fields) == 0 {
		return append([]string(nil), DefaultRegions...), nil
	}
	if len(fields) == 1 && strings.EqualFold(fields[0], "all") {
		return choices, nil
	}
	seen := map[string]bool{}
	var picked []string
	for _, f := range fields {
		key, err := pickKey(f, choices)
		if err != nil {
			return nil, err
		}
		if !seen[key] {
			seen[key] = true
			picked = append(picked, key)
		}
	}
	return picked, nil
}

func pickKey(f string, choices []string) (string, error) {
	if n, err := strconv.Atoi(f); err == nil {
		if n < 1 || n > len(choices) {
			return "", fmt.Errorf("%d is not on the list (1-%d)", n, len(choices))
		}
		return choices[n-1], nil
	}
	key := normalize(f)
	if key == "local" || key == "nearest" {
		return "local", nil
	}
	if _, ok := Presets[key]; ok {
		return key, nil
	}
	return "", fmt.Errorf("unknown region %q", f)
}

func displayName(key string) string {
	if key == "local" {
		return "Local (nearest)"
	}
	return Presets[key].Name
}

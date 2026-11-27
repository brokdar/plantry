// Command fdcgen builds the offline food catalogue that adapters/fdclocal
// embeds (foods.tsv.gz) from the USDA FoodData Central CSV downloads. It is
// a build-time tool and is not part of the plantry binary.
//
// Regenerate (USDA FDC data is public domain, CC0 1.0):
//
//	curl -LO https://fdc.nal.usda.gov/fdc-datasets/FoodData_Central_sr_legacy_food_csv_2018-04.zip
//	curl -LO https://fdc.nal.usda.gov/fdc-datasets/FoodData_Central_foundation_food_csv_2025-12-18.zip
//	cd backend && GOWORK=off go run ./tools/fdcgen \
//	  -sr <dir>/FoodData_Central_sr_legacy_food_csv_2018-04.zip \
//	  -foundation <dir>/FoodData_Central_foundation_food_csv_2025-12-18.zip \
//	  -out internal/adapters/fdclocal/foods.tsv.gz
//
// Pick the newest Foundation zip on https://fdc.nal.usda.gov/download-datasets
// that contains food.csv and food_nutrient.csv (the 2026-10-29 release lacks
// both). The raw zips are never committed.
//
// Output, one record per line, tab separated, gzip compressed:
//
//	C <category id> <category name>
//	F <fdc id> <category id> <description> <kcal> <protein g> <fat g> <carbs g> <fiber g> <sugar g> <sodium mg> <saturated fat g>
//	P <fdc id> <unit> <modifier> <grams per one unit>
//
// All nutrients are per 100 g; an empty field is a value FDC does not report.
package main

import (
	"archive/zip"
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Nutrient columns in output order, by FDC nutrient id. Energy falls back to
// the Atwater variants Foundation foods report (as adapters/fdc does).
var nutrientIDs = [][]int{{1008, 2048, 2047}, {1003}, {1004}, {1005}, {1079}, {2000}, {1093}, {1258}}

// skipCategories are SR Legacy groups that are branded or prepared meals,
// never a generic ingredient.
var skipCategories = map[string]bool{
	"Baby Foods": true, "Fast Foods": true, "Restaurant Foods": true,
	"American Indian/Alaska Native Foods": true, "Meals, Entrees, and Side Dishes": true,
}

type foodRow struct {
	id, cat, desc string
	nutr          map[int]float64
}

func main() {
	sr := flag.String("sr", "", "SR Legacy CSV zip")
	fnd := flag.String("foundation", "", "Foundation CSV zip")
	out := flag.String("out", "", "output .tsv.gz")
	flag.Parse()
	if *sr == "" || *fnd == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	cats := map[string]string{}
	units := map[string]string{}
	var foods []*foodRow
	var portions [][]string
	for _, src := range []struct {
		path, filter string
	}{{*sr, ""}, {*fnd, "foundation_food.csv"}} {
		f, p, err := readZip(src.path, src.filter, cats, units)
		if err != nil {
			log.Fatalf("%s: %v", src.path, err)
		}
		foods = append(foods, f...)
		portions = append(portions, p...)
	}
	if err := write(*out, cats, foods, portions); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %d foods, %d portions to %s", len(foods), len(portions), *out)
}

// readZip reads one FDC CSV release. filter names the CSV listing the
// release's own foods (Foundation zips also hold sample foods).
func readZip(path, filter string, cats, units map[string]string) ([]*foodRow, [][]string, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = z.Close() }()
	file := func(name string) (*csv.Reader, io.Closer, error) {
		for _, f := range z.File {
			if strings.HasSuffix(f.Name, "/"+name) {
				rc, err := f.Open()
				if err != nil {
					return nil, nil, err
				}
				r := csv.NewReader(bufio.NewReader(rc))
				r.ReuseRecord = true
				if _, err := r.Read(); err != nil { // header
					return nil, nil, err
				}
				return r, rc, nil
			}
		}
		return nil, nil, fmt.Errorf("%s missing", name)
	}
	each := func(name string, fn func(rec []string)) error {
		r, c, err := file(name)
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		for {
			rec, err := r.Read()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			fn(rec)
		}
	}

	if err := each("food_category.csv", func(r []string) { cats[r[0]] = r[2] }); err != nil {
		return nil, nil, err
	}
	if err := each("measure_unit.csv", func(r []string) { units[r[0]] = r[1] }); err != nil {
		return nil, nil, err
	}
	var keep map[string]bool
	if filter != "" {
		keep = map[string]bool{}
		if err := each(filter, func(r []string) { keep[r[0]] = true }); err != nil {
			return nil, nil, err
		}
	}
	byID := map[string]*foodRow{}
	var foods []*foodRow
	err = each("food.csv", func(r []string) {
		id, desc, cat := r[0], r[2], r[3]
		if keep != nil && !keep[id] || skipCategories[cats[cat]] || strings.Contains(desc, "moisture)") {
			return
		}
		f := &foodRow{id: id, cat: cat, desc: desc, nutr: map[int]float64{}}
		byID[id] = f
		foods = append(foods, f)
	})
	if err != nil {
		return nil, nil, err
	}
	err = each("food_nutrient.csv", func(r []string) {
		f := byID[r[1]]
		if f == nil || r[3] == "" {
			return
		}
		nid, err1 := strconv.Atoi(r[2])
		v, err2 := strconv.ParseFloat(r[3], 64)
		if err1 == nil && err2 == nil {
			f.nutr[nid] = v
		}
	})
	if err != nil {
		return nil, nil, err
	}
	var portions [][]string
	err = each("food_portion.csv", func(r []string) {
		if byID[r[1]] == nil {
			return
		}
		amount, _ := strconv.ParseFloat(r[3], 64)
		grams, err := strconv.ParseFloat(r[7], 64)
		if err != nil || grams <= 0 {
			return
		}
		if amount > 0 {
			grams /= amount
		}
		unit := units[r[4]]
		mod := r[6]
		if mod == "" {
			mod = r[5] // Foundation puts the size in portion_description
		}
		portions = append(portions, []string{r[1], clean(unit), clean(mod), num(grams)})
	})
	if err != nil {
		return nil, nil, err
	}
	var kept []*foodRow
	for _, f := range foods {
		if _, ok := energy(f.nutr); ok {
			kept = append(kept, f)
		}
	}
	return kept, portions, nil
}

func energy(n map[int]float64) (float64, bool) {
	for _, id := range nutrientIDs[0] {
		if v, ok := n[id]; ok {
			return v, true
		}
	}
	return 0, false
}

func write(path string, cats map[string]string, foods []*foodRow, portions [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	zw, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(zw) // write errors stick; Flush reports them
	ids := make([]string, 0, len(cats))
	for id := range cats {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return atoi(ids[i]) < atoi(ids[j]) })
	for _, id := range ids {
		_, _ = fmt.Fprintf(w, "C\t%s\t%s\n", id, clean(cats[id]))
	}
	sort.Slice(foods, func(i, j int) bool { return atoi(foods[i].id) < atoi(foods[j].id) })
	for _, fd := range foods {
		fields := []string{"F", fd.id, fd.cat, clean(fd.desc)}
		for _, ids := range nutrientIDs {
			s := ""
			for _, id := range ids {
				if v, ok := fd.nutr[id]; ok {
					s = num(v)
					break
				}
			}
			fields = append(fields, s)
		}
		_, _ = fmt.Fprintln(w, strings.Join(fields, "\t"))
	}
	for _, p := range portions {
		_, _ = fmt.Fprintln(w, "P\t"+strings.Join(p, "\t"))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

func clean(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\t", " ")), " ")
}

func num(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

package main

import (
	"bytes"
	"fmt"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	chartrender "github.com/go-echarts/go-echarts/v2/render"
	"github.com/labstack/echo/v4"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
)

type (
	basicStats struct {
		min, max, median, ninetyninth, ninetieth, seventyfifth int
		mean float64
	}
	xy struct {
		x int `bson:"count_unique_words"`
		y int `bson:"count_external_links"`
	}
	xys []xy
)

func (s xys) xSeries() []int {
	xs := make([]int, len(s), len(s))
	for i, v := range s {
		xs[i] = v.x
	}
	return xs
}

func (s xys) ySeries() []int {
	ys := make([]int, len(s), len(s))
	for i, v := range s {
		ys[i] = v.y
	}
	return ys
}

func (s xys) calculateLSR() func(x float64) (y float64) {
	xSum := 0.0
	ySum := 0.0
	xSqSum := 0.0
	xySum := 0.0

	xs := s.xSeries()
	ys := s.ySeries()

	for i, x := range xs {
		xSum += float64(x)
		ySum += float64(ys[i])
		xSqSum += float64(x*x)
		xySum += float64(x*ys[i])
	}

	// https://www.mathsisfun.com/data/least-squares-regression.html
	// y = mx + b
	//
	// m = mNumerator / mDenominator
	// mNumerator = n*xySum - xSum*ySum
	// mDenominator = n*xSqSum - xSum^2
	//
	// b = (ySum - m*xSum) / n


	n := float64(len(s))

	return func(x float64) (y float64) {
		m := (n*xySum - xSum*ySum)/(n*xSqSum - xSum*xSum)
		b := (ySum - m*xSum) / n

		return m*x + b
	}
}

func prescriptiveScatter(c echo.Context, data xys) error {
	/*scatter := charts.NewScatter()
	scatter.Initialization.Width = "750px"
	scatter.Renderer = newSnippetRenderer(scatter, c, scatter.Validate)

	scatter.AddJSFuncs("function initRegression() {echarts.registerTransform(ecStat.transform.regression);}")

	scatter.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{ Title: "External Links as a Function of Unique Token Count" }),
		charts.WithXAxisOpts(opts.XAxis{
			Type:        "value",
			Show:        true,
		}),
	)*/

	sort.Slice(data, func(i, j int) bool {
		if data[i].x == data[j].x {
			return data[i].y < data[j].y
		}
		return data[i].x < data[j].x
	})
	xs := data.xSeries()
	ys := data.ySeries()
	/*lsr := data.calculateLSR()
	lsrData := make([]opts.ScatterData, len(ys), len(ys))
	sd := make([]opts.ScatterData, len(ys), len(ys))*/
	/*min := 1000
	max := 0*/
	var ret [][]int
	for i, y := range ys {
		ret = append(ret, []int{xs[i], y})
		/*if xs[i] < min {min = xs[i]}
		if xs[i] > max {max = xs[i]}
		sd[i] = opts.ScatterData{Value: y}
		lsrData[i] = opts.ScatterData{Value: lsr(float64(xs[i]))}*/
	}

	return c.JSON(http.StatusOK, ret)
	/*scatter.SetXAxis(data.xSeries()).AddSeries("Links by Unique Token Count", sd).AddSeries("Regression Line", lsrData, charts.WithLineChartOpts(opts.LineChart{Smooth: true}))
	return scatter.Render(bytes.NewBuffer([]byte{}))*/
}

func distributionBarChart(c echo.Context, data []int, title, subtitle, seriesName string) error {
	bar := charts.NewBar()
	bar.Initialization.Width = "750px"
	bar.Renderer = newSnippetRenderer(bar, c, bar.Validate)

	bd, x, st := processIntData(data)
	bar.SetGlobalOptions(charts.WithTitleOpts(opts.Title{
		Title:    title,
		Subtitle: fmt.Sprintf("%s\r\nMean: %.2f, Min: %d, Median: %d, 75th %%ile: %d, 90th %%ile: %d, 99th %%ile: %d, Max: %d", subtitle, st.mean, st.min, st.median, st.seventyfifth, st.ninetieth, st.ninetyninth, st.max),
	}))

	bar.SetXAxis(x).AddSeries(seriesName, bd)

	return bar.Render(bytes.NewBuffer([]byte{}))
}

func processIntData(lengths []int) ([]opts.BarData, []int, basicStats) {
	sort.Slice(lengths, func(i, j int) bool { return lengths[i] < lengths[j] })
	var bd []opts.BarData
	m := make(map[int]int)
	st := basicStats{
		min: 1000,
		max: 0,
	}
	runningSum := 0
	for i, v := range lengths {
		switch i {
		case len(lengths) * 99 / 100:
			{
				st.ninetyninth = v//m[v]
			}
		case len(lengths) * 90 / 100:
			{
				st.ninetieth = v//m[v]
			}
		case len(lengths) * 75 / 100:
			{
				st.seventyfifth = v//m[v]
			}
		case len(lengths) * 50 / 100:
			{
				st.median = v//m[v]
			}
		}
		runningSum += m[v]
		if v < st.min {
			st.min = v
		}
		if v > st.max {
			st.max = v
		}
		if val, ok := m[v]; ok {
			m[v] = val + 1
		} else {
			m[v] = 1
		}
	}
	st.mean = math.Floor(float64(runningSum) / float64(len(lengths)))

	var x, y []int
	var xstr []string
	for k := range m {
		x = append(x, k)
	}
	sort.Slice(x, func(i, j int) bool { return x[i] < x[j] })
	for _, v := range x {
		xstr = append(xstr, strconv.Itoa(v))
		y = append(y, m[v])
		bd = append(bd, opts.BarData{Value: m[v]})
	}
	return bd, x, st
}

type snippetRenderer struct {
	ec echo.Context
	c      interface{}
	before []func()
}

func newSnippetRenderer(c interface{}, ec echo.Context, before ...func()) chartrender.Renderer {
	return &snippetRenderer{c: c, ec: ec, before: before}
}

func (r *snippetRenderer) Render(w io.Writer) error {
	for _, fn := range r.before {
		fn()
	}

	return r.ec.Render(http.StatusOK,"echart", r.c)
}
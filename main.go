package main

import (
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	listenAddr = ":8080"
)

var (
	mongoDbName = os.Getenv("MONGODB_NAME")
	mongodb *mongo.Client
	tokenColl *mongo.Collection
	tokenVectorColl *mongo.Collection
	wbColl *mongo.Collection
	addrRoot = os.Getenv("ADDR_ROOT")
)

type (
	wikibook struct {
		Id int `json:"_id,omitempty" bson:"_id" redis:"_id"`
		Title      string      `json:"title,omitempty" bson:"title" redis:"title"`
		Url        string      `json:"url,omitempty,omitempty" bson:"url" redis:"url"`
		Abstract   string      `json:"abstract,omitempty" bson:"abstract" redis:"abstract"`
		BodyText   string      `json:"body_text,omitempty" bson:"body_text" redis:"body_text"`
		BodyHtml   string      `json:"body_html,omitempty" bson:"body_html" redis:"body_html"`
		childPages []*wikibook
		ChildPageIds []int `json:"child_pages,omitempty" bson:"child_pages,omitempty" redis:"child_pages"`
		parentPage *wikibook
		ParentPageId int   `json:"parent_page,omitempty" bson:"parent_page" redis:"parent_page"`
	}
	tokenVectorDoc struct {
		Id            int   `bson:"_id" json:"id,omitempty"`
		TokenMap      map[string]int `bson:"compressed_token_vector" json:"-"`
		Similarity    float64   `json:"similarity,omitempty"`
		EuclidianNorm float64   `bson:"euclidian_norm" json:"-"`
		Title      string      `json:"title,omitempty" bson:"title" redis:"title"`
		Url        string      `json:"url,omitempty,omitempty" bson:"url" redis:"url"`
	}
	results struct{Id int `bson:"_id"`}
)

func main() {
	e := echo.New()

	setUpMiddleware(e)
	setUpRoutes(e)
	setUpTemplateRendering(e)

	connMongo(context.Background())

	log.Fatal(e.Start(listenAddr))
}

func connMongo(ctx context.Context) {
	var err error
	clientOpts := options.Client().ApplyURI(os.Getenv("MONGODB_CONN_STR"))
	mongodb, err = mongo.Connect(ctx, clientOpts)
	if err != nil {
		err = errors.Wrap(err, "connecting to mongodb")
		panic(err)
	}
	if err = mongodb.Ping(ctx, nil); err != nil {
		err = errors.Wrap(err, "pinging mongodb")
		panic(err)
	}
	wbColl = mongodb.Database(mongoDbName).Collection("wikibooks")
	tokenColl = mongodb.Database(mongoDbName).Collection("tokens")
	tokenVectorColl = mongodb.Database(mongoDbName).Collection("token_vector")
}

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func setUpTemplateRendering(e *echo.Echo) {
	t := template.New("master").Funcs(templateFuncs).Option("missingkey=zero")
	e.Renderer = &Template{
		templates: template.Must(t.ParseGlob("capstone/assets/templates/*.gohtml")),
	}
}
func setUpMiddleware(e *echo.Echo) {
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
}

func setUpRoutes(e *echo.Echo) {
	e.Static("/capstone/assets", "capstone/assets")
	e.GET("/capstone/home", handleHomepage)
	e.GET("/capstone/findtoken", findToken)
	e.GET("/capstone/findsimilar", findSimilar)
	e.GET("/capstone/charts/:chartname", handleChart)
}
func timeIt(operationName string) func() {
	t := time.Now()
	return func() {
		fmt.Println(fmt.Sprintf("%s took %d ms", operationName, time.Since(t).Milliseconds()))
	}
}
func findSimilar(c echo.Context) error {

	ctx := c.Request().Context()
	lookupIdStr := c.QueryParam("id")
	if lookupIdStr == "" {
		return c.String(http.StatusBadRequest, "invalid id -- empty")
	}
	lookupId, err := strconv.Atoi(lookupIdStr)
	if err != nil {
		err = errors.Wrap(err, "casting lookupIdStr")
		return c.String(http.StatusBadRequest, err.Error())
	}

	timer := timeIt("getting lookup token vector")
	tknVectorDocs, err := getTokenVectorDoc(ctx, lookupId)
	if err != nil {
		err = errors.Wrap(err, "getting compressed token vector")
		return c.String(http.StatusBadRequest, err.Error())
	}
	lookupDoc := tknVectorDocs[0]
	timer()
	
	type a struct{
		id int
		qty int
	}

	timer = timeIt("arranging/sorting lookup tokens")
	var arr []a
	for k, v := range lookupDoc.TokenMap {
		id, err := strconv.Atoi(k)
		if err != nil {
			err = errors.Wrap(err, "casting to int")
			log.Println(err)
			continue
		}
		arr = append(arr, a{id:id, qty: v})
	}

	sort.Slice(arr, func(i, j int) bool {
		return arr[i].qty < arr[j].qty
	})

	var ids bson.A
	for i := 0; i < len(arr)/10; i++ {
		ids = append(ids, arr[i].id)
	}
	timer()

	timer = timeIt("aggregating references")
	var res []results
	cur, err := tokenColl.Aggregate(ctx, mongo.Pipeline{
		bson.D{{"$match", bson.D{{"_id", bson.D{{"$in", ids}}}}}},
		bson.D{{"$unwind", "$references"}},
		//bson.D{{"$sort", bson.D{{"$references."}}}},
		//{{"$set", bson.D{{"doc", bson.D{{"$arrayElemAt", bson.A{"$references", 0}}}}}}},
		bson.D{{"$group", bson.D{{"_id", "$references._id"}}}},
	})
	if err != nil {
		err = errors.Wrap(err, "getting related doc ids")
		return c.String(http.StatusBadRequest, err.Error())
	}
	timer()

	timer = timeIt("cursoring references")
	if err = cur.All(ctx, &res); err != nil {
		err = errors.Wrap(err, "getting related doc ids")
		return c.String(http.StatusBadRequest, err.Error())
	}
	timer()

	timer = timeIt("getting comparison docs")
	compareDocs, err := getTokenVectorDoc(ctx, func(inp []results) (out []int) {for _, v := range inp { out = append(out, v.Id) }; return}(res)...)
	if err != nil {
		err = errors.Wrapf(err, "getting %d docs to compare", len(res))
		return c.String(http.StatusBadRequest, err.Error())
	}
	timer()

	timer = timeIt("comparing all docs")
	wg := sync.WaitGroup{}
	q := make(chan tokenVectorDoc)
	var allCompareDocs []tokenVectorDoc
	go func() {
		for val := range q {
			allCompareDocs = append(allCompareDocs, val)
			wg.Done()
		}
	}()

	for n, compareDoc := range compareDocs {
		if compareDoc.Id == lookupDoc.Id {
			continue
		}
		if n%10000 == 0 {
			fmt.Printf("%.2f\r\n", float64(n*100.0)/float64(len(res)))
		}
		wg.Add(1)
		go compare2Docs(&q, lookupDoc, compareDoc)
	}
	wg.Wait()
	timer()

	timer = timeIt("sorting results")
	sort.Slice(allCompareDocs, func(i, j int) bool {
		return allCompareDocs[i].Similarity > allCompareDocs[j].Similarity
	})
	timer()
	return c.JSON(http.StatusOK, allCompareDocs[:25])
}

func compare2Docs(q *chan tokenVectorDoc, lookupDoc tokenVectorDoc, compareDoc tokenVectorDoc) {
	dotProd := dotProduct(lookupDoc.TokenMap, compareDoc.TokenMap)

	compareDoc.Similarity = float64(dotProd) / (lookupDoc.EuclidianNorm * compareDoc.EuclidianNorm)
	compareDoc.TokenMap = nil
	*q <- compareDoc
}

func getTokenVectorDoc(ctx context.Context, lookupId... int) ([]tokenVectorDoc, error) {
	cur, err := tokenVectorColl.Aggregate(ctx, mongo.Pipeline{
		bson.D{{"$match", bson.D{{"_id", bson.D{{"$in", lookupId}}}}}},
		bson.D{{"$lookup", bson.D{{"from", "wikibooks"}, {"localField", "_id"}, {"foreignField", "_id"}, {"as", "book"}}}},
		bson.D{{"$set", bson.D{
			{"euclidian_norm", bson.D{{"$arrayElemAt", bson.A{"$book.euclidian_norm", 0}}}},
			{"url", bson.D{{"$arrayElemAt", bson.A{"$book.url", 0}}}},
			{"title", bson.D{{"$arrayElemAt", bson.A{"$book.title", 0}}}},
		}}},
		bson.D{{"$project", bson.D{
			{"compressed_token_vector", true},
			{"euclidian_norm", true},
			{"url", true},
			{"title", true},
		}}},
	})
	if err != nil {
		err = errors.Wrap(err, "getting lookup doc")
		return nil, err
	}

	var res []tokenVectorDoc
	err = cur.All(ctx, &res)
	if err != nil {
		err = errors.Wrap(err, "decoding lookup doc")
		return nil, err
	}
	return res, nil
}

func dotProduct(xm, ym map[string]int) int {
	passoverMap := make(map[string]bool)
	var x, y []int
	for k, xv := range xm {
		passoverMap[k] = true
		x = append(x, xv)
		if yv, ok := ym[k]; ok {
			y = append(y, yv)
		} else {
			y = append(y, 0)
		}
	}
	for k, yv := range ym {
		if _, ok := passoverMap[k]; ok {
			continue
		}
		x = append(x, 0)
		y = append(y, yv)
	}
	sum := 0
	for i := 0; i < len(x); i++ {
		sum += x[i]*y[i]
	}
	return sum
}

func handleChart(c echo.Context) error {
	ctx := c.Request().Context()
	switch c.Param("chartname") {
	case "prescriptive": {
		cur, err := wbColl.Aggregate(ctx, mongo.Pipeline{
			bson.D{{"$match", bson.D{{"count_external_links", bson.D{{"$gt", 0}}}}}},
			bson.D{{"$project", bson.D{{"count_unique_words", true}, {"count_external_links", true}}}},
		})
		if err != nil {
			err = errors.Wrap(err, "aggregating prescriptive data")
			log.Println(err)
			return c.HTML(http.StatusInternalServerError, "<pre>" + err.Error() + "</pre>")
		}

		var res xys
		defer cur.Close(ctx)
		for cur.Next(ctx) {
			var tmp struct {
				Id int `bson:"_id"`
				X int `bson:"count_unique_words"`
				Y int `bson:"count_external_links"`
			}
			err = cur.Decode(&tmp)
			res = append(res, xy{x: tmp.X, y: tmp.Y})
			if err != nil {
				err = errors.Wrap(err, "cursoring prescriptive data")
				log.Println(err)
				return c.HTML(http.StatusInternalServerError, "<pre>" + err.Error() + "</pre>")
			}
		}

		return prescriptiveScatter(c, res)
	}
	case "uniquewords": {
		cur, err := wbColl.Aggregate(ctx, mongo.Pipeline{
			bson.D{{"$match", bson.D{{"count_unique_words", bson.D{{"$gt", 0}}}}}},
			bson.D{{"$project", bson.D{{"count_unique_words", true}}}},
		})
		if err != nil {
			err = errors.Wrap(err, "aggregating unique token counts")
			log.Println(err)
			return c.HTML(http.StatusInternalServerError, "<pre>" + err.Error() + "</pre>")
		}

		defer cur.Close(ctx)
		var countArr []int

		for cur.Next(ctx) {
		   var res struct{
			   Id int `bson:"_id"`
			   UniqueTokens int `bson:"count_unique_words"`
		   }
		   if err = cur.Decode(&res); err != nil {
			   err = errors.Wrap(err, "cursor.Decode()")
			   log.Println(err)
			   return c.HTML(http.StatusInternalServerError, "<pre>" + err.Error() + "</pre>")
		   }
		   countArr = append(countArr, res.UniqueTokens)
		}

		return distributionBarChart(c, countArr, "Unique Word Counts", "Number of unique tokens in a document", "Unique Tokens")
	}
	case "pagecounts": {
		cur, err := wbColl.Aggregate(ctx, mongo.Pipeline{
			bson.D{{"$project", bson.D{{"parent_page", true}, {"child_pages", true}}}},
			bson.D{{"$match", bson.D{{"parent_page", bson.D{{"$eq", 0}}}}}},
			bson.D{{"$set", bson.D{{"page_count", bson.D{{"$add", bson.A{bson.D{{"$cond", bson.A{bson.D{{"$isArray", bson.A{"$child_pages"}}}, bson.D{{"$size", "$child_pages"}}, 0}}}, 1}}}}}}},
			bson.D{{"$project", bson.D{{"page_count", true}}}},
		})
		if err != nil {
			err = errors.Wrap(err, "aggregating chapter counts")
			log.Println(err)
			return c.HTML(http.StatusInternalServerError, "<pre>" + err.Error() + "</pre>")
		}

		var resp []struct{
			Id int `bson:"_id"`
			PageCount int `bson:"page_count"`
		}
		err = cur.All(ctx, &resp)
		if err != nil {
			err = errors.Wrap(err, "cursor.All()")
			log.Println(err)
			return c.HTML(http.StatusInternalServerError, "<pre>" + err.Error() + "</pre>")
		}
		var countArr []int
		for _, v := range resp {
			countArr = append(countArr, v.PageCount)
		}
		return distributionBarChart(c, countArr, "Chapter Counts", "Number of chapters in a Wikibook", "Chapter Counts")
	}
	case "externallinks": {
		match := bson.D{{"$match", bson.D{{"count_external_links", bson.D{{"$gt", 0}}}}}}
		cur, err := wbColl.Aggregate(ctx, mongo.Pipeline{
			match,
			bson.D{{"$project", bson.D{{"count_external_links", true}}}},
		})
		if err != nil {
			err = errors.Wrap(err, "aggregating external link counts")
			log.Println(err)
			return c.HTML(http.StatusInternalServerError, "<pre>" + err.Error() + "</pre>")
		}

		var resp []struct{
			Id int `bson:"_id"`
			LinkCount int `bson:"count_external_links"`
		}
		err = cur.All(ctx, &resp)
		if err != nil {
			err = errors.Wrap(err, "cursor.All()")
			log.Println(err)
			return c.HTML(http.StatusInternalServerError, "<pre>" + err.Error() + "</pre>")
		}
		var countArr []int
		for _, v := range resp {

			countArr = append(countArr, v.LinkCount)
		}
		return distributionBarChart(c, countArr, "Nonzero External Link Counts", "Number of external links in a Wikibook that contains nonzero links", "External Link Counts")
	}
	}
	return nil
}

func handleHomepage(c echo.Context) error {
	return c.Render(http.StatusOK, "homepage", map[string]interface{}{"addrRoot": addrRoot})
	return nil
}

func findToken(c echo.Context) error {
	ctx := c.Request().Context()
	searchVal := strings.ToLower(c.QueryParam("search"))
	if searchVal == "" {
		return c.String(http.StatusBadRequest, "a nonempty string must be submitted for search")
	}

	if strings.Contains(searchVal, " ") {
		return c.String(http.StatusBadRequest, "only a single word may be submitted for search")
	}

	match := bson.D{{"$match", bson.D{{"token", primitive.Regex{
		Pattern: fmt.Sprintf("^%s", searchVal),
		Options: "i",
	}}}}}
	unwind := bson.D{{"$unwind", "$references"}}
	set := bson.D{{"$set", bson.D{{"docId", "$references._id"}, {"qty", "$references.qty"}}}}
	project := bson.D{{"$project", bson.D{{"docId", true}, {"qty", true}, {"token", true}}}}
	lookup := bson.D{{"$lookup", bson.D{{"from", "wikibooks"}, {"localField", "docId"}, {"foreignField", "_id"}, {"as", "book"}}}}
	sortByQty := bson.D{{"$sort", bson.D{{"qty", -1}}}}
	finalProject := bson.D {{"$project", bson.D{{"token", true}, {"qty", true}, {"docId", true}, {"book.title", true}, {"book.url", true}}}}
	cur, err := tokenColl.Aggregate(ctx,
		mongo.Pipeline{
			match,
			unwind,
			set,
			project,
			lookup,
			sortByQty,
			finalProject,
		}, nil)
	if err != nil {
		err = errors.Wrap(err, "getting all matching documents from mongodb")
		return c.String(http.StatusInternalServerError, err.Error())
	}

	var allResults []struct {
		Id int `json:"_id" bson:"_id" redis:"_id"`
		Token string `bson:"token" json:"token" redis:"token"`
		DocId int `bson:"docId" json:"docId" redis:"docId"`
		Qty int `bson:"qty" json:"qty" redis:"qty"`
		Book []wikibook `bson:"book" json:"book" redis:"book"`
	}
	err = cur.All(ctx, &allResults)
	if err != nil {
		err = errors.Wrap(err, "cur.All()")
		return c.String(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, allResults)
}

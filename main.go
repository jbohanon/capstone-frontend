package main

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	listenAddr = ":8080"
)

var (
	mongodb *mongo.Client
	redisdb *redis.Client
	tokenColl *mongo.Collection
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
	mongodb, err = mongo.Connect(ctx, nil)
	if err != nil {
		err = errors.Wrap(err, "connecting to mongodb")
		panic(err)
	}
	if err = mongodb.Ping(ctx, nil); err != nil {
		err = errors.Wrap(err, "pinging mongodb")
		panic(err)
	}
	wbColl = mongodb.Database("wikibooks").Collection("wikibooks")
	tokenColl = mongodb.Database("wikibooks").Collection("tokens")
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

func findSimilar(c echo.Context) error {
	ctx := c.Request().Context()
	lookupId := c.QueryParam("id")
	if lookupId == "" {
		return c.String(http.StatusBadRequest, "invalid id -- empty")
	}


	return c.NoContent(http.StatusNotImplemented)
}

func handleChart(c echo.Context) error {
	ctx := c.Request().Context()
	switch c.Param("chartname") {
	case "uniquewords": {
		cur, err := wbColl.Aggregate(ctx, mongo.Pipeline{
			bson.D{{"$match", bson.D{{"tokens", bson.D{{"$ne", nil}}}}}},
			bson.D{{"$set", bson.D{{"uniquetokens", bson.D{{"$size", "$tokens"}}}}}},
			bson.D{{"$project", bson.D{{"uniquetokens", true}}}},
		})
		if err != nil {
			err = errors.Wrap(err, "aggregating unique token counts")
			log.Println(err)
			return c.HTML(http.StatusInternalServerError, "<pre>" + err.Error() + "</pre>")
		}
		var resp []struct{
			Id int `bson:"_id"`
			UniqueTokens int `bson:"uniquetokens"`
		}
		err = cur.All(ctx, &resp)
		if err != nil {
			err = errors.Wrap(err, "cursor.All()")
			log.Println(err)
			return c.HTML(http.StatusInternalServerError, "<pre>" + err.Error() + "</pre>")
		}
		var countArr []int
		for _, v := range resp {
			countArr = append(countArr, v.UniqueTokens)
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
		match := bson.D{{"$match", bson.D{{"body_html", primitive.Regex{
			Pattern: "href=\"h",
			Options: "i",
		}}}}}
		cur, err := wbColl.Aggregate(ctx, mongo.Pipeline{
			match,
			bson.D{{"$project", bson.D{{"body_html", true}}}},
			bson.D{{"$project", bson.D{{"link_count", bson.D{{"$subtract", bson.A{bson.D{{"$size", bson.D{{"$split", bson.A{"$body_html", "href=\"h"}}}}}, 1}}}}}}},
			bson.D{{"$match", bson.D{{"link_count", bson.D{{"$gt", 0}}}}}},
		})
		if err != nil {
			err = errors.Wrap(err, "aggregating external link counts")
			log.Println(err)
			return c.HTML(http.StatusInternalServerError, "<pre>" + err.Error() + "</pre>")
		}

		var resp []struct{
			Id int `bson:"_id"`
			LinkCount int `bson:"link_count"`
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
	set := bson.D{{"$set", bson.D{{"docId", "$references.id"}, {"qty", "$references.qty"}}}}
	project := bson.D{{"$project", bson.D{{"docId", true}, {"qty", true}, {"token", true}}}}
	lookup := bson.D{{"$lookup", bson.D{{"from", "wikibooks"}, {"localField", "docId"}, {"foreignField", "_id"}, {"as", "book"}}}}
	sort := bson.D{{"$sort", bson.D{{"qty", -1}}}}
	finalProject := bson.D {{"$project", bson.D{{"token", true}, {"qty", true}, {"docId", true}, {"book.title", true}, {"book.url", true}}}}
	//cur, err := tokenColl.Find(ctx, bson.D{{Key: "token", Value: fmt.Sprintf("/.*%s.*/", searchVal)}}, options.Find().SetProjection(bson.D{{Key: "references.id", Value: 1}}))
	cur, err := tokenColl.Aggregate(ctx,
		mongo.Pipeline{
			match,
			unwind,
			set,
			project,
			lookup,
			sort,
			finalProject,
		}, nil)

	if err != nil {
		err = errors.Wrap(err, "getting all matching documents from mongodb")
		return c.String(http.StatusInternalServerError, err.Error())
	}
	type res struct {
		Id primitive.ObjectID `json:"_id" bson:"_id" redis:"_id"`
		Token string `bson:"token" json:"token" redis:"token"`
		DocId int `bson:"docId" json:"docId" redis:"docId"`
		Qty int `bson:"qty" json:"qty" redis:"qty"`
		Book []wikibook `bson:"book" json:"book" redis:"book"`
	}
	var allResults []res
	err = cur.All(ctx, &allResults)
	if err != nil {
		err = errors.Wrap(err, "cur.All()")
		return c.String(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, allResults)
	//wbColl.Aggregate(ctx, bson.D{{Key: "$project", Value: "title"}, {Key: "$match", Value: bson.D{"_id"}}})
}

package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"golang.org/x/net/html"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var dbConnection *gorm.DB
var l = sync.RWMutex{}

func init() {
	db, err := gorm.Open("postgres", "host=localhost port=5432 user=xmvmpwalsagthi dbname=test password=test sslmode=disable")

	if err != nil {
		fmt.Printf("failed to connect database : %v", err)
		panic("failed to connect database")
	}

	dbConnection = db

	db.AutoMigrate(&Category{}, &User{}, &Country{}, &Chap{}, &Movie{})
}

type Model struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

type Category struct {
	Model
	Href string `gorm:"PRIMARY_KEY"`
	Name string
}

type User struct {
	Model
	Name  string
	Image string
	Href  string `gorm:"PRIMARY_KEY"`
}

//type UserMovie struct {
//	Model
//	idMovie   float32
//	idUser    float32
//	character string
//}

type Country struct {
	Model
	Code string `gorm:"PRIMARY_KEY"`
	Href string
	Name string
}

type Chap struct {
	Model
	IdMovie string
	Url     string
}

type Keyword struct {
	Text string `gorm:"PRIMARY_KEY"`
}

type Movie struct {
	Model
	NameVi            string
	NameEn            string
	Duration          string
	Quantity          string
	Resolution        string
	Language          string
	ProductionCompany string
	Content           string `gorm:"type:text"`
	Trailer           string
	Poster            string
	Status            string
	Year              int
	View              float64
	IMDb              float64
	AW                float64
	Director          []User `gorm:"many2many:movie_director_user;"`
	Actor             []User `gorm:"many2many:movie_actor_user;"`
	Error             string
	Url               string `gorm:"primary_key"`
	Keywords          []Keyword
	Chaps             []Chap     `gorm:"foreignkey:idMovie"`
	Categories        []Category `gorm:"many2many:user_categories;"`
	Country           []Country  `gorm:"many2many:movie_country;"`
	ReleaseDate       string
}

var urlsPage = map[int]map[int]string{}

func log(format string, a ...interface{}) {
	fmt.Printf(format, a...)
}

func getFailRetry(url string, numberRetry int, errBefore error) (doc *goquery.Document, err error) {
	if numberRetry <= -1 {
		return nil, errBefore
	}
	res, err := http.Get(url)

	if err != nil {
		log("Error get url : %v with error : %v", url, err)
		return getFailRetry(url, numberRetry-1, err)
	}
	defer res.Body.Close()

	doc, err = goquery.NewDocumentFromReader(res.Body)

	if err != nil {
		log("Error read document in url : %s", url)
		return getFailRetry(url, numberRetry-1, err)
	}

	if res.StatusCode != 200 {
		log("status code error: %d %s %s", res.StatusCode, res.Status, url)
		return getFailRetry(url, numberRetry-1, err)
	}

	return doc, err
}

func getUrlByPageAndPosition(page int, index int) (url string) {
	if urlsPage[page] == nil {
		url := fmt.Sprintf("http://www.phimmoi.net/phim-le/page-%v.html", page)
		document, err := getFailRetry(url, 3, nil)
		if err != nil {
			log("Error get Document by page : %v with error : %v", url, err)
			return ""
		}

		urlsPage[page] = map[int]string{}

		document.Find(".list-movie > .movie-item").Each(func(i int, s *goquery.Selection) {
			urlMovie, exits := s.Find(".block-wrapper").Attr("href")
			if !exits {
				return
			}
			l.Lock()
			urlsPage[page][i] = urlMovie
			l.Unlock()
		})
	}

	l.Lock()
	url = urlsPage[page][index-1]
	l.Unlock()

	if url == "" {
		time.Sleep(3 * time.Microsecond)
		log("retry get Document by page : %v \n", url)
		return getUrlByPageAndPosition(page, index)
	}

	return url
}

func startRunCrawlerPage(page int, maxRoutine int) {
	maxTotalMovie := page * 30
	var ch = make(chan int, maxTotalMovie)
	var wg sync.WaitGroup

	wg.Add(maxRoutine)
	for i := 0; i < maxRoutine; i++ {
		go func() {
			for {
				page, ok := <-ch
				if !ok {
					wg.Done()
					return
				}
				crawlerMovieInPage(page)
			}
		}()
	}

	for i := 0; i < maxTotalMovie; i++ {
		ch <- i
	}

	close(ch)
	wg.Wait()
}

func crawlerMovieInPage(a int) {
	page := a/30 + 1
	positionMovie := a%30 + 1
	log("start movie at %d in page %d \n", positionMovie, page)

	url := getUrlByPageAndPosition(page, positionMovie)
	if url == "" {
		log("movie at %d in page %d is null \n", positionMovie, page)
		return
	}

	crawlerMovie(url)
}

func getItemWhere(dt *goquery.Selection, dd *goquery.Selection, condition string) string {

	index := 0

	dt.Each(func(i int, selection *goquery.Selection) {

		if selection.Text() == condition {
			index = i
		}
	})

	test := &goquery.Selection{Nodes: []*html.Node{dd.Get(index)}}

	return test.Text()
}

func crawlerMovie(movieUrl string) {
	url := fmt.Sprintf("http://www.phimmoi.net/%s", movieUrl)

	document, err := getFailRetry(url, 3, nil)

	data := document.Find("iframe").AttrOr("src", "")

	fmt.Printf(data)

	if err != nil {
		log("Error get url : %v with error : %v", url, err)
		return
	}

	movieDl := document.Find(".movie-meta-info .movie-dl")

	var directors []User
	movieDl.Find(".dd-director .director").Each(func(i int, selection *goquery.Selection) {
		href, exits := selection.Attr("href")
		if !exits {
			return
		}

		directors = append(directors, User{Name: selection.Text(), Href: href})
	})

	var countries []Country

	movieDl.Find(".dd-country .country").Each(func(i int, selection *goquery.Selection) {
		href, exits := selection.Attr("href")
		if !exits {
			return
		}

		code := strings.Split(href, "/")[1]

		countries = append(countries, Country{Name: selection.Text(), Href: href, Code: code})
	})

	var actors []User

	document.Find("#list_actor_carousel .actor-profile-item").Each(func(i int, selection *goquery.Selection) {
		href, exits := selection.Attr("href")
		if !exits {
			return
		}
		image := strings.Split(selection.Find(".actor-image").AttrOr("style", "''"), "'")[1]

		actors = append(actors, User{Name: selection.Text(), Href: href, Image: image})
	})

	var categories []Category

	movieDl.Find(".dd-cat .category").Each(func(i int, selection *goquery.Selection) {
		href, exits := selection.Attr("href")
		if !exits {
			return
		}

		categories = append(categories, Category{Name: selection.Text(), Href: href})
	})

	var keywords []Keyword

	movieDl.Find(".tag-list .tag-item").Each(func(i int, selection *goquery.Selection) {

		keywords = append(keywords, Keyword{Text: selection.Text()})
	})

	domDt := movieDl.Find(".movie-dt")
	domDd := movieDl.Find(".movie-dd")

	view, _ := strconv.ParseFloat(getItemWhere(domDt, domDd, "Lượt xem:"), 32)
	year, _ := strconv.Atoi(getItemWhere(domDt, domDd, "Năm:"))
	imdb, _ := strconv.ParseFloat(getItemWhere(domDt, domDd, "Điểm IMDb:"), 32)
	aw, _ := strconv.ParseFloat(getItemWhere(domDt, domDd, "Điểm AW:"), 32)

	content, _ := document.Find("#film-content").Html()
	movie := &Movie{
		Status:            movieDl.Find(".status").Text(),
		NameEn:            document.Find(".title-2").Text(),
		NameVi:            document.Find("a.title-1").Text(),
		Year:              year,
		ReleaseDate:       getItemWhere(domDt, domDd, "Ngày ra rạp:"),
		Duration:          getItemWhere(domDt, domDd, "Thời lượng:"),
		Quantity:          getItemWhere(domDt, domDd, "Chất lượng:"),
		Resolution:        getItemWhere(domDt, domDd, "Độ phân giải:"),
		Language:          getItemWhere(domDt, domDd, "Ngôn ngữ:"),
		ProductionCompany: getItemWhere(domDt, domDd, "Công ty SX:"),
		View:              view,
		IMDb:              imdb,
		AW:                aw,
		Director:          directors,
		Country:           countries,
		Actor:             actors,
		Content:           content,
		Categories:        categories,
		Poster:            document.Find(".movie-l-img img").AttrOr("src", ""),
		Url:               url,
		//Trailer:           document.Find("#trailer-preroll-container iframe").AttrOr("src", ""),
		Keywords: keywords,
	}
	dbConnection.FirstOrCreate(&movie, Movie{Url: url})

}

func main() {

	startRunCrawlerPage(136, 20)

	defer dbConnection.Close()
}

package crawler

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	mu = sync.RWMutex{}
)

type (
	CommentChanel struct {
		PostCount int
		Date      string
		User      User
		Comment   string
	}
	User struct {
		Username     string
		RegisterDate string
	}
	PageChanel struct {
		Url string
		Doc *goquery.Selection
	}
	PageToCrawlChanel struct {
		Url     string
		pageNum int
	}
)

func Crawl(ctx context.Context, cf Configuration) {
	lastPage, err := GetPageNumber(cf.URL)
	if err != nil {
		return
	}
	urlchanel := make(chan PageToCrawlChanel, lastPage)
	pageCrawled := make(chan *PageChanel, lastPage)
	go func(ctx context.Context) {
		pgawg := &sync.WaitGroup{}
		for i := 0; i <= cf.MaximumWorker; i++ {
			pgawg.Add(1)
			go CrawlSpecifiPage(ctx, pgawg, urlchanel, pageCrawled)
		}
		pgawg.Wait()
		close(pageCrawled)
	}(ctx)
	for i := 1; i <= lastPage; i++ {
		urlchanel <- PageToCrawlChanel{
			Url:     fmt.Sprintf("%s&page=%d", cf.URL, i),
			pageNum: i,
		}
	}
	close(urlchanel)

}

// reuse bypass function form github.com/lnquy/vozer
func ByPassVoz(url string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Cookie", "vflastvisit=1535954670; vflastactivity=0; vfforum_view=d99e85613f547374e9db4f942bf6192fb611ae2aa-1-%7Bi-17_i-1535954671_%7D; _ga=GA1.2.144936460.1535954673; _gid=GA1.2.1737523081.1535954673; _gat_gtag_UA_351630_1=1")
	req.Header.Add("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/68.0.3440.106 Safari/537.36")
	return req, nil
}
func getHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 10 * time.Second,
	}
}
func GetPageNumber(url string) (int, error) {
	req, err := ByPassVoz(url)
	if err != nil {
		return 0, err
	}

	resp, err := getHTTPClient().Do(req)
	if err != nil || resp.StatusCode/200 != 1 {
		return 0, errors.Wrap(err, fmt.Sprintf("can't get content form %s", url))
	}
	firstpage, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return 0, err
	}
	//Page 1 of 7
	t := firstpage.Find("div.neo_column .main table").First().Find("tbody td .vbmenu_control").First().Text()
	if t == "" {
		// if only one page
		return 1, nil
	}
	pagenav := strings.Split(t, " ")
	if len(pagenav) == 0 {
		return 0, errors.New("can't get page number, this page have only one page or error")
	}
	resp.Body.Close()
	return strconv.Atoi(pagenav[len(pagenav)-1])
}
func CrawlSpecifiPage(ctx context.Context, wg *sync.WaitGroup, inputch <-chan PageToCrawlChanel, pagech chan<- *PageChanel) {
	for {
		select {
		case <-ctx.Done():
			logrus.Infof("crawler is terminated by user")
			return
		case urlc, ok := <-inputch:
			if !ok {
				logrus.Infof("crawler page &d done", urlc.pageNum)
			}
			s := CrawlPage(urlc.Url)
			if s == nil {
				logrus.Println("get data fail")
			}
			pagech <- &PageChanel{Url: urlc.Url, Doc: s}
			wg.Done()

		}
	}
}
func CrawlPage(url string) *goquery.Selection {
	respon, err := ByPassVoz(url)
	if err != nil {
		logrus.Error("can't connect to &s", url)
		return nil
	}
	doc, err := goquery.NewDocumentFromReader(respon.Body)
	if err != nil {
		logrus.Error("can't create new dom from &s", url)
		return nil
	}
	s := doc.Find("table .tborder .voz-postbit")
	return s
}
func GetCommnet(ctx context.Context, pagech <-chan *PageChanel) {
	for {
		select {
		case <-ctx.Done():
			logrus.Infof("stop")
			return
		case s, ok := <-pagech:
			if !ok {
				return
			}
			c := CommentChanel{}
			s.Doc.Each(func(i int, selection *goquery.Selection) {
				postCountStr, _ := selection.Find("tbody tr").First().Find("td div").First().Find("a").First().Attr("name")
				postCount, _ := strconv.Atoi(postCountStr)
				c.PostCount = postCount
				c.Date = selection.Find("tbody tr").First().Find("td .normal").Last().Find("a").Text()

			})
		}
	}
}
func WirteDataToFile(data interface{}, url string) {
	f, err := os.OpenFile(fmt.Sprintf("resutl%s-%s", url, time.Now()), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	text, _ := json.Marshal(data)
	if _, err = f.WriteString(string(text)); err != nil {
		panic(err)
	}
}

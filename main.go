package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/gocolly/colly/v2"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/image/webp"
)

type Article struct {
	Name         string
	Link         string
	ImageLink    string
	Date         string
	Author       string
	Catagory     string
	CommentCount int
}

var articleImagePrefix = 10000 //alphanumerically sort images

func isWithinTimeframe(dateString string, numDays int) bool {
	// Parse the date string using a specific layout
	layout := "January 2, 2006"
	t, err := time.Parse(layout, dateString)
	if err != nil {
		// Handle the error
		return false
	}

	// Get the current time
	now := time.Now()

	// Calculate the difference between the current time and the parsed time
	diff := now.Sub(t)

	dayDifference := numDays * 1000 * 1000 * 1000 * 60 * 60 * 24

	// Check if the difference is less than or equal to 7 days
	if diff <= time.Duration(dayDifference) {
		return true
	} else {
		return false
	}
}

func getCommentCount(commentString string) int {
	// Remove the "Comments" text and any leading/trailing whitespace
	// commentString 1,890 Comments -> 1,890
	commentCountString := strings.Split(strings.Replace(commentString, ",", "", -1), " ")[0] // "1890"

	// Convert the string to an integer
	commentCount, intParsingError := strconv.Atoi(commentCountString) // 7
	if intParsingError != nil {
		// Handle the error
		fmt.Println(intParsingError)
	}
	return commentCount

}

func writeToDB(articles []Article) {
	dbPath := "docarticles.db"
	// Ensure database directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatal(err)
	}
	database, dbInitError := sql.Open("sqlite3", dbPath)
	if dbInitError != nil {
		log.Fatal(dbInitError)
	}
	fmt.Println("Opened new db docarticles")
	defer database.Close()

	// Create table if it doesn't exist first
	statement, createTableError := database.Prepare("CREATE TABLE IF NOT EXISTS articles (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, link TEXT, imagelink TEXT, date DATE, author TEXT, catagory TEXT, commentcount INTEGER)")
	if createTableError != nil {
		log.Fatal(createTableError)
	}
	statement.Exec()

	// Now try to drop the table (it's safe because we know it exists)
	_, err := database.Exec("DELETE FROM articles")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Cleared existing articles")

	statement.Exec()
	statement, createArticleError := database.Prepare("INSERT INTO articles (name, link, imagelink, date, author, catagory, commentcount) VALUES (?, ?, ?, ?, ?, ?, ?)")
	if createArticleError != nil {
		log.Fatal(createArticleError)
	}
	fmt.Println("Inserting all articles now...")

	for _, article := range articles {
		fmt.Println(article)
		statement.Exec(article.Name, article.Link, article.ImageLink, article.Date, article.Author, article.Catagory, article.CommentCount)
	}
	fmt.Println("Inserted all articles")

	fmt.Println("Selecting all articles")
	rows, selectError := database.Query("SELECT id, name, link, imagelink, date, author, catagory, commentcount FROM articles")

	if selectError != nil {
		log.Fatal(selectError)
	}
	var id int
	var name string
	var link string
	var imagelink string
	var date string
	var author string
	var catagory string
	var commentcount int

	for rows.Next() {
		err := rows.Scan(&id, &name, &link, &imagelink, &date, &author, &catagory, &commentcount)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(id, ": ", name, ". How many comments does this article have? ", commentcount)
	}
	rowErr := rows.Err()
	if rowErr != nil {
		log.Fatal(err)
	}
	defer rows.Close()

}

// splitText splits the input text into lines considering max characters and word boundaries
func splitText(text string, maxCharsPerLine int) []string {
	words := strings.Split(text, " ")
	var lines []string
	currentLine := ""
	for _, word := range words {
		if len(currentLine)+len(word)+1 <= maxCharsPerLine { // +1 for space
			currentLine += word + " "
		} else {
			lines = append(lines, currentLine)
			currentLine = word + " "
		}
	}
	lines = append(lines, currentLine)
	return lines
}

func getArticleFromDOCArticle(e *colly.HTMLElement) Article {
	article := Article{
		Name:         e.ChildText(".entry-header .entry-title a"),
		Link:         e.ChildAttr(".entry-header .entry-title a", "href"),
		ImageLink:    e.ChildAttr(".meta-image a img", "src"),
		Date:         e.ChildText(".entry-header .entry-meta .date .updated"),
		Author:       e.ChildText(".entry-header .entry-meta .author .author .fn a"),
		Catagory:     e.ChildText(".entry-header .entry-meta .meta-category"),
		CommentCount: getCommentCount(e.ChildText(".entry-header .entry-meta .comments a")),
	}
	return article
}

func saveArticleImage(imageLink string, cwd string) string {
	// Add the Article to the slice
	fmt.Println(" getting image from link")
	fmt.Println(imageLink)
	response, imgGetError := http.Get(imageLink)
	if imgGetError != nil {
		log.Fatal(imgGetError)
	}
	defer response.Body.Close()

	//open a file for writing
	localImageNameSplit := strings.Split(imageLink, "/")
	localImageName := localImageNameSplit[len(localImageNameSplit)-1]
	imagePath := fmt.Sprintf("%d%s", articleImagePrefix, localImageName)
	fullImageFilepath := filepath.Join(cwd, "docImages", imagePath)

	fmt.Println(fullImageFilepath)
	imgFile, fileCreateError := os.Create(fullImageFilepath)

	if fileCreateError != nil {
		log.Fatal(fileCreateError)
	}
	defer imgFile.Close()

	// Use io.Copy to just dump the response body to the file. This supports huge files
	_, imgCopyError := io.Copy(imgFile, response.Body)
	if imgCopyError != nil {
		log.Fatal(imgCopyError)
	}
	return fullImageFilepath

}
func saveArticleImageWithText(imageFilePath string, articleName string) string {
	fullImageFilepathText := filepath.Join(filepath.Dir(imageFilePath), "text_added_"+filepath.Base(imageFilePath))
	fmt.Println(fullImageFilepathText)
	// at this point, we can use that file as an input to add text
	existingImageFile, err := os.Open(imageFilePath)
	if err != nil {
		fmt.Println("Error opening image file:", err)
	}
	defer existingImageFile.Close()

	var rawImageData image.Image
	var decodeErr error
	imageFilePathSplit := strings.Split(imageFilePath, ".")
	if imageFilePathSplit[len(imageFilePathSplit)-1] == "png" {
		rawImageData, decodeErr = png.Decode(existingImageFile)
	} else if imageFilePathSplit[len(imageFilePathSplit)-1] == "jpg" || imageFilePathSplit[len(imageFilePathSplit)-1] == "jpeg" {
		rawImageData, decodeErr = jpeg.Decode(existingImageFile)
	} else if imageFilePathSplit[len(imageFilePathSplit)-1] == "gif" {
		rawImageData, decodeErr = gif.Decode(existingImageFile)
	} else if imageFilePathSplit[len(imageFilePathSplit)-1] == "webp" {
		rawImageData, decodeErr = webp.Decode(existingImageFile)
	}
	if decodeErr != nil {
		fmt.Println("Error decoding image:", decodeErr)
	}
	dc := gg.NewContextForImage(rawImageData)

	// Define text position and content
	fontSize := 10.0
	textX := 4.0
	textY := 20.0
	maxCharsPerLine := 30
	fontPath := "./fonts/DejaVuSans.ttf"
	if err := dc.LoadFontFace(fontPath, fontSize); err != nil {
		panic(err)
	}
	dc.SetRGB(0.8, 0.8, 0.45) // Set text color to black

	// Split the text into lines considering max characters and word boundaries
	articleStringLines := splitText(articleName, maxCharsPerLine)

	// Adjust starting Y position for each line
	lineHeight := fontSize + 5 // Adjust line height based on font size

	for _, line := range articleStringLines {
		// Draw each line of text
		dc.DrawString(line, textX, textY)
		textY += lineHeight
	}

	// Create a new file for the modified image
	fmt.Println("attempting to create image with text at")
	fmt.Println(fullImageFilepathText)
	newFile, err := os.Create(fullImageFilepathText)
	if err != nil {
		fmt.Println("Error creating new image file:", err)
	}
	defer newFile.Close()

	// Encode the modified image (using gg context) as PNG and save it
	err = png.Encode(newFile, dc.Image())
	if err != nil {
		fmt.Println("Error encoding and saving image:", err)
	}

	fmt.Println("Successfully added text and saved new image:", fullImageFilepathText)

	return fullImageFilepathText
}

func main() {
	fmt.Println("Running...")
	if len(os.Args) < 2 {
		fmt.Println("Program halted early. Must supply at least one day. Example: \"go run . 2\" for 2 days of articles")
		os.Exit(4)
		return
	}
	userDays, dayArgConversionError := strconv.Atoi(os.Args[1])
	if dayArgConversionError != nil {
		log.Fatal(dayArgConversionError)
		os.Exit(4)
		return
	}
	if userDays < 1 {
		fmt.Println("Must supply at least one day. Example: \"go run . 2\" for 2 days of articles")
		os.Exit(4)
		return
	}
	cwd, wdErr := os.Getwd()
	if wdErr != nil {
		fmt.Println("Error getting current directory:", wdErr)
		return
	}
	osClearError := os.RemoveAll(filepath.Join(cwd, "docImages"))
	if osClearError != nil {
		log.Fatal("Failed to remove remenants of previous program run. Please delete docImages folder manually if it exists.")
	}
	subfolderError := os.MkdirAll(filepath.Join(cwd, "docImages"), 0755)
	if subfolderError != nil {
		fmt.Println("Error getting current directory:", subfolderError)
		return
	}
	fmt.Println("created directory")

	var articles []Article
	var reachedEndDate = false

	// Instantiate default collector
	c := colly.NewCollector(
		// Visit only domains
		colly.AllowedDomains("doctorofcredit.com", "www.doctorofcredit.com"),
	)
	// set a human-like user agent
	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3"
	// Set a delay between requests
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 1,
		RandomDelay: 600 * time.Millisecond,
	})

	// On every a element which has href attribute call callback
	c.OnHTML(".vce-loop-wrap article", func(e *colly.HTMLElement) {
		article := getArticleFromDOCArticle(e)
		imageFilePath := saveArticleImage(article.ImageLink, cwd)
		_ = saveArticleImageWithText(imageFilePath, article.Name)
		articles = append(articles, article)

		articleImagePrefix++
		if !isWithinTimeframe(article.Date, userDays) {
			reachedEndDate = true
		}

	})
	c.OnHTML("#vce-pagination .next", func(e *colly.HTMLElement) {
		if !reachedEndDate {
			c.Visit(e.Attr("href"))
		}
	})

	// Before making a request print "Visiting ..."
	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting", r.URL.String())
	})

	// Start scraping on your initial website
	collyStartupError := c.Visit("https://doctorofcredit.com/")
	if collyStartupError != nil {
		fmt.Println("Error starting up Colly:", collyStartupError.Error())
	}

	articleJson, jsonConversionErr := json.Marshal(articles)
	if jsonConversionErr != nil {
		log.Fatal(jsonConversionErr)
	}

	//write to OS
	os.WriteFile("articles.json", articleJson, 0644)
	fmt.Println("Write to file successful")

	// write to DB
	writeToDB(articles)

}

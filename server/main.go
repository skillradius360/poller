package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

type App struct {
	mongo *mongo.Database
	
	redis *redis.Client
}

var wsUpgrader = websocket.Upgrader{

	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Email        string             `bson:"email" json:"email"`
	
	PasswordHash string             `bson:"passwordHash" json:"-"`
	CreatedAt    time.Time          `bson:"createdAt" json:"createdAt"`
}

type Poll struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`

	OwnerID    primitive.ObjectID `bson:"ownerId" json:"ownerId"`

	
	Question   string             `bson:"question" json:"question"`
	Options    []string           `bson:"options" json:"options"`
	CreatedAt  time.Time          `bson:"createdAt" json:"createdAt"`
	
	Votes      []int64            `bson:"-" json:"votes"`
	TotalVotes int64              `bson:"-" json:"totalVotes"`
}

type Vote struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	PollID      primitive.ObjectID `bson:"pollId"`
	OptionIndex int                `bson:"optionIndex"`
	CreatedAt   time.Time          `bson:"createdAt"`
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type createPollRequest struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

type voteRequest struct {
	OptionIndex int `json:"optionIndex"`
}

type pollUpdate struct {
	Type       string  `json:"type"`
	PollID     string  `json:"pollId"`
	Votes      []int64 `json:"votes"`
	TotalVotes int64   `json:"totalVotes"`
}

func main() {
	ctx := context.Background()
	app := &App{
		mongo: connectMongo(ctx),
		redis: connectRedis(ctx),
	}

	if err := app.ensureIndexes(ctx); err != nil {
		log.Fatal(err)
	}

	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	router.GET("/health", app.health)

	router.POST("/api/auth/signup", app.signup)
	router.POST("/api/auth/login", app.login)

	router.GET("/api/polls", app.listPolls)

	router.GET("/api/polls/:id", app.getPoll)
	router.POST("/api/polls/:id/vote", app.vote)
	router.GET("/ws/:id", app.watchPoll)

	protected := router.Group("/api", app.requireAuth)
	protected.POST("/polls", app.createPoll)

	port := env("PORT", "8080")
	log.Printf("Server running on http://localhost:%s", port)
	log.Fatal(router.Run(":" + port))
}

func connectMongo(ctx context.Context) *mongo.Database {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(env("MONGO_URI", "mongodb://localhost:27017")))
	if err != nil {
		log.Fatal(err)
	}


	for attempt := 1; attempt <= 30; attempt++ {
		if err = client.Ping(ctx, nil); err == nil {

			log.Println("MongoDB connected")
			return client.Database(env("MONGO_DB", "voting_app"))

		}
		log.Printf("Waiting for MongoDB (%d/30): %v", attempt, err)

		time.Sleep(2 * time.Second)
	}

	log.Fatal("Could not connect to MongoD lolool:", err)

	return nil

}



func connectRedis(ctx context.Context) *redis.Client {

	client := redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "localhost:6379")})


	
	for attempt := 1; attempt <= 30; attempt++ {
	
		if err := client.Ping(ctx).Err(); err == nil {
	
			log.Println("Redis connected")
	
			return client
	
			} else {
	
				log.Printf("Waiting for Redis (%d/30): %v", attempt, err)
	
			}
	
			time.Sleep(2 * time.Second)
	
		}


	log.
	Fatal("Could not connect to Redis")
	
	return nil
}


func (a *App) ensureIndexes(ctx context.Context) error {
	_, err := a.mongo.Collection("users").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	_, err = a.mongo.Collection("votes").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "pollId", Value: 1}},
	})
	return err
}

func (a *App) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (a *App) signup(c *gin.Context) {
	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	email, password, err := cleanCredentials(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create user"})
		return
	}

	user := User{Email: email, PasswordHash: string(hash), CreatedAt: time.Now().UTC()}
	result, err := a.mongo.Collection("users").InsertOne(c.Request.Context(), user)
	if mongo.IsDuplicateKeyError(err) {
		c.JSON(http.StatusConflict, gin.H{"error": "Email already registered"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create user"})
		return
	}

	user.ID = result.InsertedID.(primitive.ObjectID)
	token, err := a.createSession(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create session"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"token": token, "user": user})
}

func (a *App) login(c *gin.Context) {
	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	email, password, err := cleanCredentials(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user User
	err = a.mongo.Collection("users").FindOne(c.Request.Context(), bson.M{"email": email}).Decode(&user)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	token, err := a.createSession(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

func (a *App) createPoll(c *gin.Context) {
	var req createPollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	question, options, err := cleanPoll(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	poll := Poll{
		OwnerID:   c.MustGet("userID").(primitive.ObjectID),
		Question:  question,
		Options:   options,
		CreatedAt: time.Now().UTC(),
		Votes:     make([]int64, len(options)),
	}

	result, err := a.mongo.Collection("polls").InsertOne(c.Request.Context(), poll)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create poll"})
		return
	}

	poll.ID = result.InsertedID.(primitive.ObjectID)
	a.seedRedisCounts(c.Request.Context(), poll.ID, len(poll.Options))
	c.JSON(http.StatusCreated, poll)
}

func (a *App) listPolls(c *gin.Context) {
	cursor, err := a.mongo.Collection("polls").Find(
		c.Request.Context(),
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load polls"})
		return
	}
	defer cursor.Close(c.Request.Context())

	polls := []Poll{}
	for cursor.Next(c.Request.Context()) {
		var poll Poll
		if err := cursor.Decode(&poll); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read poll"})
			return
		}
		a.attachCounts(c.Request.Context(), &poll)
		polls = append(polls, poll)
	}

	c.JSON(http.StatusOK, polls)
}

func (a *App) getPoll(c *gin.Context) {
	poll, err := a.loadPoll(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Poll not found"})
		return
	}
	c.JSON(http.StatusOK, poll)
}

func (a *App) vote(c *gin.Context) {
	poll, err := a.loadPoll(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Poll not found"})
		return
	}

	var req voteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}
	if req.OptionIndex < 0 || req.OptionIndex >= len(poll.Options) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid option index"})
		return
	}

	vote := Vote{PollID: poll.ID, OptionIndex: req.OptionIndex, CreatedAt: time.Now().UTC()}
	if _, err := a.mongo.Collection("votes").InsertOne(c.Request.Context(), vote); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save vote"})
		return
	}

	if err := a.redis.HIncrBy(c.Request.Context(), votesKey(poll.ID), strconv.Itoa(req.OptionIndex), 1).Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not update live counts"})
		return
	}

	update, err := a.pollUpdate(c.Request.Context(), poll)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load vote counts"})
		return
	}

	payload, _ := json.Marshal(update)
	a.redis.Publish(c.Request.Context(), updatesChannel(poll.ID), payload)
	c.JSON(http.StatusOK, update)
}

func (a *App) watchPoll(c *gin.Context) {
	poll, err := a.loadPoll(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Poll not found"})
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	initial, err := a.pollUpdate(c.Request.Context(), poll)
	if err == nil {
		conn.WriteJSON(initial)
	}

	pubsub := a.redis.Subscribe(c.Request.Context(), updatesChannel(poll.ID))
	defer pubsub.Close()

	for message := range pubsub.Channel() {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, []byte(message.Payload)); err != nil {
			return
		}
	}
}

func (a *App) loadPoll(ctx context.Context, id string) (Poll, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return Poll{}, err
	}

	var poll Poll
	err = a.mongo.Collection("polls").FindOne(ctx, bson.M{"_id": objectID}).Decode(&poll)
	if err != nil {
		return Poll{}, err
	}

	a.attachCounts(ctx, &poll)
	return poll, nil
}

func (a *App) attachCounts(ctx context.Context, poll *Poll) {
	votes, total, err := a.countsFromRedis(ctx, poll.ID, len(poll.Options))
	if err != nil {
		log.Printf("Redis count read failed for poll %s: %v", poll.ID.Hex(), err)
		votes, total = make([]int64, len(poll.Options)), 0
	}

	poll.Votes = votes
	poll.TotalVotes = total
}

func (a *App) pollUpdate(ctx context.Context, poll Poll) (pollUpdate, error) {
	votes, total, err := a.countsFromRedis(ctx, poll.ID, len(poll.Options))
	if err != nil {
		return pollUpdate{}, err
	}

	return pollUpdate{
		Type:       "vote_update",
		PollID:     poll.ID.Hex(),
		Votes:      votes,
		TotalVotes: total,
	}, nil
}

func (a *App) countsFromRedis(ctx context.Context, pollID primitive.ObjectID, optionCount int) ([]int64, int64, error) {
	values, err := a.redis.HGetAll(ctx, votesKey(pollID)).Result()
	if err != nil {
		return nil, 0, err
	}

	if len(values) == 0 {
		if err := a.rebuildCounts(ctx, pollID, optionCount); err != nil {
			return nil, 0, err
		}
		values, err = a.redis.HGetAll(ctx, votesKey(pollID)).Result()
		if err != nil {
			return nil, 0, err
		}
	}

	votes := make([]int64, optionCount)
	var total int64
	for i := range votes {
		count, _ := strconv.ParseInt(values[strconv.Itoa(i)], 10, 64)
		votes[i] = count
		total += count
	}
	return votes, total, nil
}

func (a *App) rebuildCounts(ctx context.Context, pollID primitive.ObjectID, optionCount int) error {
	counts := make(map[string]interface{}, optionCount)
	for i := 0; i < optionCount; i++ {
		counts[strconv.Itoa(i)] = int64(0)
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"pollId": pollID}}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$optionIndex", "count": bson.M{"$sum": 1}}}},
	}
	cursor, err := a.mongo.Collection("votes").Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var row struct {
			OptionIndex int   `bson:"_id"`
			Count       int64 `bson:"count"`
		}
		if err := cursor.Decode(&row); err != nil {
			return err
		}
		if row.OptionIndex >= 0 && row.OptionIndex < optionCount {
			counts[strconv.Itoa(row.OptionIndex)] = row.Count
		}
	}

	return a.redis.HSet(ctx, votesKey(pollID), counts).Err()
}

func (a *App) seedRedisCounts(ctx context.Context, pollID primitive.ObjectID, optionCount int) {
	if err := a.rebuildCounts(ctx, pollID, optionCount); err != nil {
		log.Printf("Could not seed Redis counts for poll %s: %v", pollID.Hex(), err)
	}
}

func (a *App) createSession(ctx context.Context, userID primitive.ObjectID) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}

	token := hex.EncodeToString(tokenBytes)
	err := a.redis.Set(ctx, "session:"+token, userID.Hex(), 7*24*time.Hour).Err()
	return token, err
}

func (a *App) requireAuth(c *gin.Context) {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Login required"})
		return
	}

	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	userIDHex, err := a.redis.Get(c.Request.Context(), "session:"+token).Result()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid session"})
		return
	}

	userID, err := primitive.ObjectIDFromHex(userIDHex)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid session"})
		return
	}

	c.Set("userID", userID)
	c.Next()
}

func cleanCredentials(req authRequest) (string, string, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	password := strings.TrimSpace(req.Password)

	if !strings.Contains(email, "@") || len(email) > 120 {
		return "", "", errors.New("Enter a valid email")
	}
	if len(password) < 6 || len(password) > 72 {
		return "", "", errors.New("Password must be 6-72 characters")
	}

	return email, password, nil
}

func cleanPoll(req createPollRequest) (string, []string, error) {
	question := strings.TrimSpace(req.Question)
	if len(question) < 3 || len(question) > 180 {
		return "", nil, errors.New("Question must be 3-180 characters")
	}

	seen := map[string]bool{}
	optionsList := make([]string, 0, len(req.Options))
	for _, raw := range req.Options {
		option := strings.TrimSpace(raw)
		key := strings.ToLower(option)
		if option == "" || seen[key] {
			continue
		}
		if len(option) > 80 {
			return "", nil, errors.New("Options must be 80 characters or less")
		}
		seen[key] = true
		optionsList = append(optionsList, option)
	}

	if len(optionsList) < 2 || len(optionsList) > 8 {
		return "", nil, errors.New("Poll must contain 2-8 unique options")
	}

	return question, optionsList, nil
}

func votesKey(pollID primitive.ObjectID) string {
	return "poll:" + pollID.Hex() + ":votes"
}

func updatesChannel(pollID primitive.ObjectID) string {
	return "poll:" + pollID.Hex() + ":updates"
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

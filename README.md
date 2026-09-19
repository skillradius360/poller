# PulsePoll


Flow: create poll -> share link -> audience votes -> everyone sees live results.

## Tech used as per the requirement

- Frontend: React + Vite
- Backend: Go + Gin
- Database: MongoDB
- Realtime/counts/pub-sub: Redis
- Transport: WebSocket



## Run locally

```bash
docker compose up --build
```

Links : ---->>>>

- Frontend: http://localhost:3000
- API: http://localhost:8080
- Health: http://localhost:8080/health

## How to use

1. sign up or log in.
2. create a poll.
3. share the generated  link as in the frontend.
4. voters open the link and vote.
5. results update live without refresh.

Poll creation requires login. Voting is public because the shared link is meant for the audience.

## API

- `POST /api/auth/signup`
- `POST /api/auth/login`
- `GET /api/polls`
- `POST /api/polls` requires `Authorization: Bearer <token>` implemented a basic jwt auth and store them in redis 
- `GET /api/polls/:id`
- `POST /api/polls/:id/vote`
- `GET /ws/:id`



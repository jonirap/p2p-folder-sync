# תיעוד מפתחים - P2P Folder Sync

מערכת מבוזרת לסנכרון קבצים בזמן אמת, מיושמת ב-Go עם ארכיטקטורת peer-to-peer והצפנה מקצה לקצה.

## דרישות מקדימות

- Go 1.21+
- SQLite 3.x
- Git
- Make
- Docker (אופציונלי)

## התקנה והגדרה

```bash
# שכפול הפרויקט
git clone https://github.com/jonirap/p2p-folder-sync
cd p2p-folder-sync

# התקנת תלויות
go mod download
go mod verify

# בדיקת תקינות
make check
```

## בנייה

### בנייה רגילה

```bash
make build
```

### בנייה מותאמת לייצור

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/p2p-sync ./cmd/p2p-sync
```

### בנייה עם מידע גרסה

```bash
VERSION=$(git describe --tags)
go build -ldflags="-X main.version=${VERSION}" -o bin/p2p-sync ./cmd/p2p-sync
```

### Cross-compilation

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o bin/p2p-sync-linux ./cmd/p2p-sync

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o bin/p2p-sync-darwin ./cmd/p2p-sync

# Windows
GOOS=windows GOARCH=amd64 go build -o bin/p2p-sync.exe ./cmd/p2p-sync
```

## בנייה והפעלה עם Docker

```bash
# בנייה
docker build -t jonirap/p2p-folder-sync:latest .

# הפעלה
docker run -d \
  --name p2p-sync \
  -v /path/to/sync:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  jonirap/p2p-folder-sync:latest

# העלאה ל-registry
docker push jonirap/p2p-folder-sync:latest
```

## הרצת מבחנים

### כל המבחנים

```bash
make test
```

### מבחנים עם כיסוי קוד

```bash
make test-coverage
open coverage.html
```

### מבחני יחידה בלבד

```bash
./test/run_system_tests.sh --unit-only
```

### מבחני אינטגרציה בלבד

```bash
./test/run_system_tests.sh --integration-only
```

### מבחנים ללא Docker (מהיר)

```bash
./test/run_system_tests.sh --fast
```

### מבחן ספציפי

```bash
go test -v ./internal/sync/... -run TestVectorClock
```

### מבחנים עם race detector

```bash
go test -race ./...
```

### מבחנים מפורטים

```bash
go test -v ./...
```

### benchmarks

```bash
go test -bench=. ./internal/hashing/
```

## פקודות שימושיות נוספות

### ניקוי

```bash
make clean
```

### פורמט קוד

```bash
make fmt
```

### lint

```bash
make lint
```

### בדיקה מלאה (format + lint + test)

```bash
make check
```

## הפעלה מקומית

```bash
# הפעלה בסיסית
./bin/p2p-sync

# עם קובץ הגדרות
./bin/p2p-sync -config config/config.yaml

# עם logs מפורטים
LOG_LEVEL=debug ./bin/p2p-sync

# עם פורטים מותאמים אישית
P2P_PORT=9090 P2P_DISCOVERY_PORT=9091 ./bin/p2p-sync
```

## Debugging

### עם Delve

```bash
# התקנת delve
go install github.com/go-delve/delve/cmd/dlv@latest

# debug אפליקציה
dlv debug ./cmd/p2p-sync -- -config config/config.yaml

# debug מבחן
dlv test ./internal/sync -- -test.run TestVectorClock
```

## מבנה הפרויקט

```
p2p-folder-sync/
├── cmd/p2p-sync/          # נקודת כניסה
├── internal/              # קוד פנימי
│   ├── sync/              # מנוע סנכרון
│   ├── network/           # שכבת רשת
│   ├── database/          # SQLite
│   ├── filesystem/        # פעולות קבצים
│   ├── crypto/            # הצפנה
│   ├── chunking/          # חלוקת קבצים
│   ├── hashing/           # hash
│   └── compression/       # דחיסה
└── test/                  # מבחנים
    ├── unit/
    ├── integration/
    └── system/
```

## משתני סביבה

```bash
P2P_SYNC_FOLDER=/path/to/sync    # תיקיית סנכרון
P2P_PORT=8080                    # פורט תקשורת
P2P_DISCOVERY_PORT=8081          # פורט גילוי
LOG_LEVEL=debug                  # רמת logs (debug/info/warn/error)
PEERS=192.168.1.10:8080          # peers ידועים
P2P_TESTING_MODE=true            # מצב מבחן
```

## Git Workflow

```bash
# branch חדש
git checkout -b feature/feature-name

# שינויים
git add .
git commit -m "feat: description"

# לפני push
make check

# push
git push origin feature/feature-name
```

---

**Repository**: https://github.com/jonirap/p2p-folder-sync

**Docker Image**: `jonirap/p2p-folder-sync:latest`

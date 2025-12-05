# מדריך שימוש - P2P Folder Sync

## תיאור

מערכת לסנכרון תיקיות בין מחשבים ברשת מקומית באופן מבוזר, ללא שרת מרכזי. המערכת מבוססת על ארכיטקטורת peer-to-peer עם הצפנה מקצה לקצה.

## התקנה

```bash
docker pull <registry>/p2p-sync:latest
```

## הפעלה

### הפעלת node בסיסי

```bash
docker run -d \
  --name p2p-sync \
  -v /path/to/sync:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  <registry>/p2p-sync:latest
```

### פרמטרים

- `/path/to/sync` - נתיב התיקייה לסנכרון במחשב המקומי
- `8080` - פורט תקשורת בין nodes
- `8081/udp` - פורט גילוי nodes ברשת

### הפעלת nodes נוספים

יש להריץ את אותה פקודה על מחשבים נוספים ברשת. ה-nodes יזהו זה את זה אוטומטית.

## הגדרות נוספות

### חיבור ידני ל-nodes

במקרה שה-nodes אינם ברשת משנה אחת:

```bash
docker run -d \
  --name p2p-sync \
  -v /path/to/sync:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  -e PEERS="192.168.1.10:8080,192.168.1.11:8080" \
  <registry>/p2p-sync:latest
```

### הפעלה אוטומטית

להפעלה אוטומטית בעת אתחול:

```bash
docker run -d \
  --restart=unless-stopped \
  --name p2p-sync \
  -v /path/to/sync:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  <registry>/p2p-sync:latest
```

## בדיקת תקינות

### בדיקת logs

```bash
docker logs p2p-sync
```

### Logs מפורטים

```bash
docker run -d \
  --name p2p-sync \
  -v /path/to/sync:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  -e LOG_LEVEL=debug \
  <registry>/p2p-sync:latest
```

## פתרון תקלות

**Nodes לא מוצאים זה את זה**: ודא שהמחשבים באותה רשת או השתמש בהגדרה ידנית עם משתנה `PEERS`.

**קבצים לא מסתנכרנים**: בדוק logs באמצעות `docker logs p2p-sync`.

**שגיאות הרשאות**: ודא הרשאות קריאה/כתיבה לתיקייה `/path/to/sync`.

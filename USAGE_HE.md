# מדריך שימוש מפורט - P2P Folder Sync

## תיאור

מערכת לסנכרון תיקיות בין מחשבים באופן מבוזר (peer-to-peer), ללא שרת מרכזי. המערכת תומכת בהצפנה מקצה לקצה וניתן להפעיל אותה ברשת מקומית או דרך האינטרנט.

## דרישות מקדימות

1. Docker מותקן על המחשב ([הוראות התקנה](https://docs.docker.com/engine/install/))
2. הרשאות להפעלת Docker
3. גישה לרשת (אינטרנט או רשת מקומית)

## התקנה

```bash
docker pull jonirap/p2p-folder-sync:latest
```

---

## תרחיש 1: שני מחשבים באותה רשת מקומית (הפשוט ביותר)

זהו התרחיש הפשוט ביותר - שני מחשבים מחוברים לאותו נתב (router) בבית או במשרד.

### שלב 1: הפעל על מחשב A

```bash
# החלף /home/user/Documents במיקום התיקייה שברצונך לסנכרן
docker run -d \
  --restart=unless-stopped \
  --name p2p-sync \
  -v /home/user/Documents:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  -e LOG_LEVEL=info \
  jonirap/p2p-folder-sync:latest
```

### שלב 2: המתן 10 שניות ולאחר מכן הפעל על מחשב B

```bash
# החלף /home/user/Documents במיקום התיקייה שברצונך לסנכרן
docker run -d \
  --restart=unless-stopped \
  --name p2p-sync \
  -v /home/user/Documents:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  -e LOG_LEVEL=info \
  jonirap/p2p-folder-sync:latest
```

### שלב 3: בדוק שהמחשבים מצאו זה את זה

על כל מחשב, הרץ:

```bash
docker logs p2p-sync
```

אם אתה רואה הודעות כמו "Connected to peer" או "Peer discovered", המערכת עובדת!

**זהו! הקבצים יסתנכרנו אוטומטית.**

---

## תרחיש 2: שני מחשבים ברשתות שונות (דרך האינטרנט)

זה מתאים כאשר רוצים לסנכרן בין הבית למשרד, או בין מקומות שונים.

### מידע חשוב

כדי שמחשבים יתחברו דרך האינטרנט, אחד מהם חייב להיות נגיש מהאינטרנט. נקרא לו "מחשב A" (השרת).

### שלב 1: גלה את כתובת ה-IP המקומית של מחשב A

**ב-Linux/Mac:**

```bash
ip addr show | grep "inet " | grep -v 127.0.0.1
```

**ב-Windows (PowerShell):**

```powershell
ipconfig | findstr IPv4
```

תראה משהו כמו: `192.168.1.100` - זאת כתובת ה-IP **המקומית** שלך.

### שלב 2: גלה את כתובת ה-IP הציבורית של מחשב A

היכנס לאתר [https://whatismyip.com](https://whatismyip.com) מהמחשב או הרץ:

```bash
curl ifconfig.me
```

תקבל משהו כמו: `82.45.123.89` - זאת כתובת ה-IP **הציבורית** שלך.

### שלב 3: הגדר Port Forwarding בנתב (Router) של מחשב A

זהו השלב החשוב ביותר! אתה צריך להגדיר שהפורטים 8080 ו-8081 יופנו למחשב A.

1. **היכנס לממשק הניהול של הנתב:**

   - פתח דפדפן והקלד: `192.168.1.1` או `192.168.0.1` (תלוי בנתב)
   - הקלד שם משתמש וסיסמה (בדרך כלל כתוב על הנתב או במדריך)

2. **חפש את ההגדרות הבאות:**

   - "Port Forwarding" או "Virtual Server" או "NAT"
   - בנתבי ביזק: "יישומים ושירותים" → "Port Forwarding"
   - בנתבי HOT: "Advanced" → "NAT" → "Virtual Server"

3. **הוסף שני חוקים:**

   **חוק 1 - פורט תקשורת:**

   - External Port: `8080`
   - Internal Port: `8080`
   - Internal IP: `192.168.1.100` (ה-IP המקומי של מחשב A)
   - Protocol: `TCP`

   **חוק 2 - פורט גילוי:**

   - External Port: `8081`
   - Internal Port: `8081`
   - Internal IP: `192.168.1.100` (ה-IP המקומי של מחשב A)
   - Protocol: `UDP`

4. **שמור את ההגדרות והפעל מחדש את הנתב אם נדרש**

### שלב 4: בדוק שה-Port Forwarding עובד

מ**מחשב אחר** (או מהטלפון עם נתוני סלולר), בדוק:

```bash
telnet 82.45.123.89 8080
```

אם אתה מקבל חיבור (ולא timeout), זה עובד!

### שלב 5: הפעל על מחשב A (השרת)

```bash
# החלף את הנתיב לתיקייה שברצונך לסנכרן
docker run -d \
  --restart=unless-stopped \
  --name p2p-sync \
  -v /home/user/Documents:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  -e LOG_LEVEL=info \
  jonirap/p2p-folder-sync:latest
```

### שלב 6: הפעל על מחשב B (הלקוח) - ציין את ה-IP הציבורי של מחשב A

```bash
# החלף 82.45.123.89 בכתובת ה-IP הציבורית של מחשב A
# החלף את הנתיב לתיקייה שברצונך לסנכרן
docker run -d \
  --restart=unless-stopped \
  --name p2p-sync \
  -v /home/user/Documents:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  -e PEERS="82.45.123.89:8080" \
  -e LOG_LEVEL=info \
  jonirap/p2p-folder-sync:latest
```

**שים לב:** שינינו רק שורה אחת - הוספנו `-e PEERS="82.45.123.89:8080"` עם כתובת ה-IP הציבורית.

### שלב 7: בדוק חיבור

על מחשב B, הרץ:

```bash
docker logs p2p-sync
```

אם אתה רואה "Connected to peer 82.45.123.89:8080", זה עובד!

---

## תרחיש 3: שלושה מחשבים או יותר

### שלושה מחשבים באותה רשת מקומית

פשוט הרץ את הפקודה מתרחיש 1 על כל מחשב. הם ימצאו זה את זה אוטומטית.

### שלושה מחשבים ברשתות שונות

**אופציה 1: מחשב A כ"שרת מרכזי"**

1. עקוב אחר תרחיש 2 להגדרת מחשב A
2. על מחשבים B ו-C, השתמש באותה פקודה עם `PEERS` המפנה למחשב A
3. מחשבים B ו-C יתחברו למחשב A, ודרכו גם ימצאו זה את זה

**אופציה 2: כל מחשב נגיש מהאינטרנט**

1. הגדר Port Forwarding על כל מחשב (שלבים 1-4 מתרחיש 2)
2. על כל מחשב, הוסף את ה-IPs הציבוריים של המחשבים האחרים:

```bash
docker run -d \
  --restart=unless-stopped \
  --name p2p-sync \
  -v /home/user/Documents:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  -e PEERS="82.45.123.89:8080,91.102.34.56:8080" \
  -e LOG_LEVEL=info \
  jonirap/p2p-folder-sync:latest
```

---

## פקודות שימושיות

### בדיקת סטטוס

```bash
docker logs p2p-sync
```

### צפייה ב-logs בזמן אמת

```bash
docker logs -f p2p-sync
```

### עצירת המערכת

```bash
docker stop p2p-sync
```

### הפעלה מחדש

```bash
docker restart p2p-sync
```

### מחיקה מוחלטת (שמור על הקבצים המסונכרנים)

```bash
docker stop p2p-sync
docker rm p2p-sync
docker volume rm p2p-sync-db
```

### הפעלה עם logs מפורטים (לאבחון בעיות)

הוסף למשתנים: `-e LOG_LEVEL=debug`

---

## פתרון בעיות נפוצות

### 1. "Nodes לא מוצאים זה את זה" ברשת מקומית

**בדוק:**

- האם שני המחשבים מחוברים לאותו נתב (Wi-Fi או כבל)?
- האם חומת האש (Firewall) חוסמת את הפורטים 8080/8081?

**פתרון:**

**ב-Linux (Ubuntu/Debian):**

```bash
sudo ufw allow 8080/tcp
sudo ufw allow 8081/udp
```

**ב-Windows:**

1. חפש "Windows Defender Firewall"
2. לחץ על "Advanced settings"
3. לחץ על "Inbound Rules" → "New Rule"
4. בחר "Port" → "TCP" → הוסף 8080
5. חזור על זה עבור UDP 8081

**ב-macOS:**

```bash
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --add /usr/local/bin/docker
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --unblock /usr/local/bin/docker
```

### 2. "Cannot connect to peer" דרך האינטרנט

**בדוק את Port Forwarding:**

1. היכנס ל: [https://www.yougetsignal.com/tools/open-ports/](https://www.yougetsignal.com/tools/open-ports/)
2. הקלד את ה-IP הציבורי שלך ופורט 8080
3. לחץ "Check"
4. אם כתוב "Closed", ה-Port Forwarding לא מוגדר נכון

**בדוק שהמערכת רצה:**

```bash
docker ps | grep p2p-sync
```

אם אין פלט, המערכת לא רצה. הרץ:

```bash
docker logs p2p-sync
```

### 3. "Permission denied" בגישה לתיקייה

הבעיה: Docker אין הרשאות לתיקייה.

**פתרון ב-Linux:**

```bash
# החלף /home/user/Documents בנתיב שלך
sudo chown -R $USER:$USER /home/user/Documents
chmod -R 755 /home/user/Documents
```

### 4. הקבצים מסתנכרנים לאט

זה תלוי במהירות הרשת ובגודל הקבצים. המערכת מצפינה הכל לפני השליחה.

**בדוק מהירות:**

- רשת מקומית: צריך להיות מהיר (עשרות MB לשנייה)
- דרך האינטרנט: תלוי במהירות ההעלאה של מחשב A

**שיפור ביצועים:**

```bash
# הוסף למשתנים (אם הרשת שלך מהירה):
-e P2P_MAX_CONNECTIONS=10 \
-e P2P_CHUNK_SIZE=1048576
```

### 5. כתובת ה-IP הציבורית משתנה (IP דינמי)

רוב ספקי האינטרנט מספקים IP דינמי שמשתנה מדי פעם.

**פתרונות:**

1. **השתמש ב-Dynamic DNS (DDNS):** שירותים כמו [DuckDNS](https://www.duckdns.org) או [No-IP](https://www.noip.com) - בחינם!
2. **בקש IP סטטי מהספק:** לפעמים זה בתשלום נוסף
3. **שמור סקריפט שבודק שינויים:** הרץ סקריפט שבודק את ה-IP כל שעה ומעדכן

**דוגמה לשימוש ב-DuckDNS:**

במקום:

```bash
-e PEERS="82.45.123.89:8080"
```

השתמש:

```bash
-e PEERS="myhome.duckdns.org:8080"
```

### 6. "Container already exists"

אם אתה מנסה להריץ שוב ומקבל שגיאה:

```bash
docker rm -f p2p-sync
# ולאחר מכן הרץ מחדש את פקודת ההפעלה
```

---

## טיפים לאבטחה

1. **אל תחשוף את המערכת לאינטרנט ללא צורך:** אם אפשר, השתמש ב-VPN במקום Port Forwarding
2. **השתמש בחומת אש:** הגבל גישה לפורט 8080 רק מכתובות IP מוכרות
3. **עדכן באופן קבוע:** `docker pull jonirap/p2p-folder-sync:latest`
4. **גיבוי:** המערכת לא מחליפה גיבויים! השתמש גם ב-backup נוסף

---

## דוגמה מלאה: הגדרה בין בית למשרד

### במשרד (מחשב A - השרת)

1. גלה IP מקומי: `192.168.1.50`
2. גלה IP ציבורי: `82.45.123.89`
3. הגדר Port Forwarding בנתב: 8080 (TCP) + 8081 (UDP) → 192.168.1.50
4. הרץ:

```bash
docker run -d \
  --restart=unless-stopped \
  --name p2p-sync \
  -v /home/user/work-files:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  -e LOG_LEVEL=info \
  jonirap/p2p-folder-sync:latest
```

### בבית (מחשב B - הלקוח)

```bash
docker run -d \
  --restart=unless-stopped \
  --name p2p-sync \
  -v /home/user/work-files:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  -e PEERS="82.45.123.89:8080" \
  -e LOG_LEVEL=info \
  jonirap/p2p-folder-sync:latest
```

**זהו!** הקבצים בתיקיית `work-files` יסתנכרנו אוטומטית בין הבית למשרד.

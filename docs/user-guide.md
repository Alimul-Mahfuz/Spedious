# Spedious User Guide: How to Test Your APIs Like a Pro

> **Who is this for?** Everyone! Even if you have **never written a single line of Go code** in your life, you can master Spedious in 5 minutes. Spedious runs as an easy-to-use tool (just like `curl` or `git`), and test instructions are written in simple, readable YAML text.

---

## Table of Contents

1. [Explain It Like I'm 10: How Spedious Works](#1-explain-it-like-im-10-how-spedious-works)
2. [The 6 Building Blocks (The Robot Army Metaphor)](#2-the-6-building-blocks-the-robot-army-metaphor)
3. [Installing & Running the App (No Coding Needed)](#3-installing--running-the-app-no-coding-needed)
4. [Step-by-Step: Writing Your First Scenario](#4-step-by-step-writing-your-first-scenario)
5. [The Robot's Secret Powers (Variables & Functions)](#5-the-robots-secret-powers-variables--functions)
6. [Response Extractors (The Robot's Notebook)](#6-response-extractors-the-robots-notebook)
7. [Assertions (The Robot's Inspection Checklist)](#7-assertions-the-robots-inspection-checklist)
8. [Load Shapes (How Your Robots Enter the Store)](#8-load-shapes-how-your-robots-enter-the-store)
   - [Closed Model (The Revolving Door)](#closed-model-the-revolving-door)
   - [Open Model (The School Bus Schedule)](#open-model-the-school-bus-schedule)
9. [Operating the CLI (Just 2 Commands!)](#9-operating-the-cli-just-2-commands)
10. [Reading the Report Card (Test Results)](#10-reading-the-report-card-test-results)
11. [Exporting Results (JSON & CSV)](#11-exporting-results-json--csv)
12. [Common Pitfalls & Troubleshooting](#12-common-pitfalls--troubleshooting)

---

## 1. Explain It Like I'm 10: How Spedious Works

Imagine you opened the most popular **lemonade stand** in your neighborhood:

* When **1 friend** walks up, you pour a cup of lemonade in **5 seconds**. Easy!
* But what happens when the entire school—**1,000 thirsty kids**—rushes your stand at the exact same second?
  * Will you run out of cups?
  * Will your table collapse under the pressure?
  * Will the kid at the back of the line have to wait 20 minutes?

**Spedious is your Robot Customer Simulator.**

Instead of waiting for real humans to crash your website or API on launch day, you tell Spedious:
> *"Hey Spedious! Send 500 robot friends to my online store. Have them log in, search for a toy, and buy it. Time every single click with a precision stopwatch and tell me if anything breaks!"*

Spedious releases the robots, collects every millisecond of timing data, and gives your website a clean **Pass or Fail report card**.

---

## 2. The 6 Building Blocks (The Robot Army Metaphor)

When you write a test file in Spedious, you are simply giving instructions to your robot helpers using 6 basic concepts:

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Virtual User (VU)  → A robot friend with a stopwatch     │
│ 2. Step               → One action (e.g. "Buy lemonade")    │
│ 3. Variables          → The robot's backpack (saved notes)  │
│ 4. Extractor          → Writing down the order ticket #     │
│ 5. Assertion          → Quality check ("Was it delicious?") │
│ 6. Threshold          → The final report card grade         │
└─────────────────────────────────────────────────────────────┘
```

1. **Virtual User (VU) = A Robot Friend**  
   If you set `vus: 10`, you have 10 robots visiting your website at the same time.
2. **Step = An Action**  
   A step is one thing the robot does. For example: "Go to `/items` and look at the toys."
3. **Variables = The Robot's Memory**  
   Save common information like the website address (`baseUrl: http://localhost:8080`) so you never have to re-type it.
4. **Extractor = The Robot's Notebook**  
   When the robot logs in, the server gives it a VIP Wristband (Token). The robot writes that token down in its notebook so it can enter the VIP lounge in the next step!
5. **Assertion = The Quality Checklist**  
   "Did the cashier say Hello (HTTP status `200`)?"  
   "Did I get my item in under 500 milliseconds?"  
   If yes $\rightarrow$ Green Checkmark (`✓`). If no $\rightarrow$ Red Flag (`✗`).
6. **Threshold = The Final Report Card**  
   "To pass the test, 95% of orders must take less than 1 second, and the error rate must be under 1%." If your server fails the goal, Spedious fails with a red alarm.

---

## 3. Installing & Running the App (No Coding Needed)

Spedious is a single file called an executable binary (`spedious.exe` on Windows, or `spedious` on Mac/Linux). 

You **never** have to compile code or learn Go. 

### Step 1: Open Terminal / Command Prompt
Navigate to where Spedious is stored:
```powershell
cd E:\MyProject\Spedious
```

### Step 2: Check It Works
Type this command:
```powershell
.\bin\spedious.exe version
```
Spedious will proudly say:
```text
spedious version 0.1.0-dev
```

You are ready to roll!

---

## 4. Step-by-Step: Writing Your First Scenario

Create a plain text file named `my_first_test.yaml`:

```yaml
version: 1
name: my_first_lemonade_stand_test

# Where the robots should go
variables:
  baseUrl: https://httpbin.org

# What the robots should do
scenarios:
  customer_visit:
    load:
      executor: constant-users  # Keep a steady group of robots
      vus: 5                    # 5 robot friends
      duration: 10s             # Visit for 10 seconds
    steps:
      - request:
          name: check_menu
          method: GET
          url: ${baseUrl}/get
        assert:
          - status == 200       # Did the page load successfully?
          - latency < 1s        # Was it faster than 1 second?
```

That's it! In less than 20 lines of simple English-like text, you wrote a complete multi-user load test.

---

## 5. The Robot's Secret Powers (Variables & Functions)

Robots don't just repeat the exact same thing; they can generate magic random data on the fly using `${...}`:

| Magic Word | What It Does (Metaphor) | Example |
|---|---|---|
| `${uuid()}` | Generates a guaranteed 100% unique secret ID | `Idempotency-Key: ${uuid()}` |
| `${random(1, 100)}` | Rolls a 100-sided dice for random quantities | `cups: ${random(1, 10)}` |
| `${randomString(8)}` | Makes up a random 8-letter username | `username: "kid_${randomString(8)}"` |
| `${now()}` | Looks at the clock and writes down the exact time | `orderTime: "${now()}"` |
| `${timestamp()}` | Milliseconds since the year 1970 | `clock: ${timestamp()}` |
| `${vu.id}` | The robot's personal jersey number (1, 2, 3...) | `email: "robot_${vu.id}@school.com"` |
| `${vu.iteration}` | How many times this robot has completed the loop | `round_${vu.iteration}` |
| `${status}` | The status code of the previous step | `${status} == 200` |
| `${latency}` | How many milliseconds the previous step took | `${latency} < 500` |

---

## 6. Response Extractors (The Robot's Notebook)

When your robot orders food, the kitchen gives them an **Order Ticket ID** (like `#512`). How does the robot remember this number so it can pick up the food later?

**Answer: Extractors!**

```yaml
- request:
    name: place_order
    method: POST
    url: ${baseUrl}/api/orders
    body:
      flavor: "Strawberry"
  extract:
    # Look at the JSON receipt and copy "id" into the robot's notebook under ${myOrderId}
    myOrderId: { jsonpath: "$.id" }

# In the very next step, the robot uses ${myOrderId}!
- request:
    name: pickup_order
    method: GET
    url: ${baseUrl}/api/orders/${myOrderId}
  assert:
    - status == 200
```

---

## 7. Assertions (The Robot's Inspection Checklist)

An assertion is a question your robot asks after every step. If the answer is NO, the robot marks it as an **Error**.

```yaml
assert:
  # 1. Did the website say "All Good" (Status 200)?
  - status == 200

  # 2. Or was it one of our accepted codes?
  - status in [200, 201]

  # 3. Did it reply in under half a second?
  - latency < 500ms

  # 4. Does the text contain what we expected?
  - body contains "Thank you for your order"

  # 5. Check a specific piece of JSON data:
  - jsonpath $.orderStatus == "ready"
  - jsonpath $.totalItems > 0
```

---

## 8. Load Shapes (How Your Robots Enter the Store)

Spedious gives you two ways to send robots into your system:

### Closed Model (The Revolving Door)
*Imagine a revolving door at a department store.* Only a fixed number of people fit inside at once.
- **`constant-users`**: You tell 20 robots to go shopping. Whenever a robot finishes, it immediately shops again until the timer runs out.
  ```yaml
  load:
    executor: constant-users
    vus: 20
    duration: 30s
  ```
- **`per-vu-iterations`**: Exactly 10 robots each buy 1 item and then go home.
  ```yaml
  load:
    executor: per-vu-iterations
    vus: 10
    iterations: 1
  ```
- **`ramping-users`**: Start with 5 robots, then gradually invite 50 robots, then slow down.
  ```yaml
  load:
    executor: ramping-users
    stages:
      - { duration: 10s, target: 10 }   # Start gentle
      - { duration: 20s, target: 50 }   # Rush hour!
      - { duration: 10s, target: 0 }    # Closing time
  ```

---

### Open Model (The School Bus Schedule)
*Imagine a school bus dropping off 30 kids every 10 seconds.*
The bus does **not** care if the lemonade stand is slow or fast. The kids arrive on schedule!

If your server gets slow, a huge line forms outside. Measuring this line is called **preventing Coordinated Omission**—it guarantees that true waiting room delays are never hidden from your charts!

```yaml
load:
  executor: constant-arrival-rate
  rate: 30                  # 30 new customers every second
  timeUnit: 1s
  duration: 1m
  maxVUs: 200               # Max room capacity
```

If the store runs out of room, Spedious logs **Dropped Iterations**, letting you know your servers are completely overwhelmed!

---

## 9. Operating the CLI (Just 2 Commands!)

You only ever need to know two commands:

### Command 1: "Check My Homework" (`validate`)
Before running any test, ask Spedious to check for typos or formatting errors:
```powershell
.\bin\spedious.exe validate my_first_test.yaml
```
Output:
```text
✓ Scenario "my_first_lemonade_stand_test" (v1) is valid
```

### Command 2: "Release the Robots!" (`run`)
Run the test:
```powershell
.\bin\spedious.exe run my_first_test.yaml
```

#### Change Your Mind? Override on the Fly!
You don't even have to open the text file to change the test:
```powershell
# "Make it 50 robots instead of 5!"
.\bin\spedious.exe run my_first_test.yaml --vus 50

# "Run it for 2 minutes!"
.\bin\spedious.exe run my_first_test.yaml --duration 2m

# "Stop immediately if anything breaks!"
.\bin\spedious.exe run my_first_test.yaml --abort-on-fail
```

---

## 10. Reading the Report Card (Test Results)

When your test finishes, Spedious prints a beautiful summary:

```text
  ==================== TEST RESULTS SUMMARY ====================
  Total Duration        : 5.00s
  Iterations            : 149 (29.8/s)     <-- Total shopping trips completed
  Total Requests        : 149 (29.8/s)     <-- Total clicks made
  Errors                : 0                <-- 0 mistakes! Perfection!
  Data Received         : 2.3 KB

  --- Latency Metrics (How fast was the stand?) ---
  http_req_duration.....: min=0.00ms med=0.51ms avg=0.80ms max=44.93ms
  percentiles...........: p90=0.81ms p95=1.06ms p99=12.10ms

  --- Network Timing Breakdown ---
  * blocked.............: avg=12.73ms      <-- Waiting for a free phone line
  * connecting..........: avg=1.14ms       <-- Saying hello on the phone
  * waiting (TTFB)......: avg=0.93ms       <-- Kitchen cooking time
  * receiving...........: avg=0.65ms       <-- Delivery time

  --- Thresholds (The Report Card) ---
  ✓ [PASS] p(95) < 50ms                    <-- Passed with flying colors!
  ✓ [PASS] rate < 0.01
  ==============================================================
  STATUS: PASSED
```

### What does `p95` mean?
Imagine 100 students taking a test. If the **p95** is 1.06ms, it means **95 out of 100 students finished their lemonade in 1.06ms or faster**! Only 5 were slightly slower. It is the gold standard for measuring real user happiness.

---

## 11. Exporting Results (JSON & CSV)

Want to graph your results in Excel or save them for your boss?

### Export a Pretty JSON Report Card:
```powershell
.\bin\spedious.exe run my_test.yaml --summary-export report.json
```

### Export a CSV Spreadsheet:
```powershell
.\bin\spedious.exe run my_test.yaml --out csv=metrics.csv
```
This generates an Excel-ready spreadsheet with columns for `timestamp`, `active_vus`, `rps`, and `p95_duration_ms` over every second of the test!

---

## 12. Common Pitfalls & Troubleshooting

### 1. "YAML syntax error"
* **The Culprit**: You forgot quotes around a JSONPath bracket!
* **Wrong**: `id: { jsonpath: $[0].id }`
* **Right**: `id: { jsonpath: "$[0].id" }`

### 2. "Connection refused"
* **The Culprit**: Your website or server is not turned on! Make sure your API is running and listening on the port (e.g. `http://localhost:5233` or `http://localhost:8080`).

### 3. "STATUS: FAILED (Thresholds breached)"
* **The Culprit**: Your test worked, but your website was too slow or had errors, failing the threshold goal you set. Check the **Step Breakdown** to see which endpoint was taking too long!

---

*Happy Load Testing! Now go release your robot army with Spedious!* 🚀

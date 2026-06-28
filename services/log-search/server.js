const http = require('http')
const fs = require('fs')
const path = require('path')
const readline = require('readline')

const PORT = 8200
const INTERNAL_KEY = process.env.INTERNAL_API_KEY || 'ob-jurnal-internal-2026'

const FLOWISE_LOG_DIR = path.join(__dirname, '../../packages/server/logs')
const LOG_FILES = [
    { source: 'telegram', file: path.join(process.env.HOME || '/root', '.pm2/logs/go-telegram-error.log') },
    { source: 'whatsapp', file: path.join(process.env.HOME || '/root', '.pm2/logs/go-wa-error.log') },
    { source: 'flowise', file: path.join(process.env.HOME || '/root', '.pm2/logs/alazhar-agentic-error.log') }
]

function readLinesFromFile(filePath) {
    return new Promise((resolve) => {
        if (!fs.existsSync(filePath)) return resolve([])
        const lines = []
        const rl = readline.createInterface({ input: fs.createReadStream(filePath), crlfDelay: Infinity })
        rl.on('line', (line) => {
            if (line.trim()) lines.push(line)
        })
        rl.on('close', () => resolve(lines))
        rl.on('error', () => resolve(lines))
    })
}

// Convert "2026-06-27" → matches "2026/06/27" and "2026-06-27"
function dateMatches(line, date) {
    if (!date) return true
    const slashed = date.replace(/-/g, '/')
    return line.includes(date) || line.includes(slashed)
}

async function searchLogs(query, date) {
    const results = []
    const q = query.toLowerCase()

    // Flowise server logs (hourly files)
    for (let h = 0; h < 24; h++) {
        const hour = h.toString().padStart(2, '0')
        const filePath = path.join(FLOWISE_LOG_DIR, `server.log.${date}-${hour}`)
        const lines = await readLinesFromFile(filePath)
        for (const line of lines) {
            if (line.toLowerCase().includes(q)) {
                results.push({ source: 'flowise', line })
            }
        }
    }

    // PM2 service logs
    for (const { source, file } of LOG_FILES) {
        const lines = await readLinesFromFile(file)
        for (const line of lines) {
            if (dateMatches(line, date) && line.toLowerCase().includes(q)) {
                results.push({ source, line })
            }
        }
    }

    return results
}

const server = http.createServer(async (req, res) => {
    res.setHeader('Content-Type', 'application/json')
    res.setHeader('Access-Control-Allow-Origin', '*')
    res.setHeader('Access-Control-Allow-Headers', 'X-Internal-Key, Content-Type')

    if (req.method === 'OPTIONS') {
        res.writeHead(204)
        res.end()
        return
    }

    if (req.headers['x-internal-key'] !== INTERNAL_KEY) {
        res.writeHead(401)
        res.end(JSON.stringify({ error: 'unauthorized' }))
        return
    }

    const url = new URL(req.url, `http://localhost:${PORT}`)

    if (url.pathname === '/logs/search' && req.method === 'GET') {
        const q = url.searchParams.get('q') || ''
        const date = url.searchParams.get('date') || ''
        if (!q) {
            res.writeHead(400)
            res.end(JSON.stringify({ error: 'query required' }))
            return
        }
        try {
            const results = await searchLogs(q, date)
            res.writeHead(200)
            res.end(JSON.stringify({ query: q, date, total: results.length, results }))
        } catch (e) {
            res.writeHead(500)
            res.end(JSON.stringify({ error: e.message }))
        }
        return
    }

    res.writeHead(404)
    res.end(JSON.stringify({ error: 'not found' }))
})

server.listen(PORT, '127.0.0.1', () => {
    console.log(`log-search listening on :${PORT}`)
})

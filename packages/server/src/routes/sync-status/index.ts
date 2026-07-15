import express from 'express'
import { exec } from 'child_process'

const router = express.Router()

function parseLogLine(raw: string): { time: string; message: string; level: string } {
    const match = raw.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.\d+)?Z\s+(.*)$/)
    if (!match) return { time: '', message: raw, level: 'INFO' }
    const message = match[2].replace(/^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2} /, '')
    const level = /fail|error|403|404|refused/i.test(message) ? 'ERROR' : /warn/i.test(message) ? 'WARN' : 'INFO'
    return { time: match[1] + 'Z', message, level }
}

// GET /logs — last 7 days, max 500 lines
router.get('/logs', (req, res) => {
    const lines = Math.min(parseInt((req.query.lines as string) || '200'), 500)
    exec(`docker logs ob-sync-indexer --timestamps --since 168h --tail ${lines} 2>&1`, { timeout: 10000 }, (err, stdout) => {
        if (err && !stdout) return res.status(503).json({ error: 'Gagal baca log: ' + err.message })
        const parsed = stdout
            .split('\n')
            .filter(Boolean)
            .map((line) => parseLogLine(line))
            .reverse()
        res.json({ logs: parsed, total: parsed.length })
    })
})

// GET /status
router.get('/status', (req, res) => {
    exec(
        `docker inspect ob-sync-indexer --format '{"running":{{.State.Running}},"startedAt":"{{.State.StartedAt}}","status":"{{.State.Status}}"}'`,
        { timeout: 5000 },
        (err, stdout) => {
            if (err) return res.status(503).json({ error: 'Container tidak ditemukan' })
            try {
                res.json(JSON.parse(stdout.trim()))
            } catch {
                res.status(500).json({ error: 'Parse error' })
            }
        }
    )
})

// POST /clear-logs — truncate Docker log file (safe: just empties the file)
router.post('/clear-logs', (req, res) => {
    exec(`docker inspect --format='{{.LogPath}}' ob-sync-indexer`, { timeout: 5000 }, (err, logPath) => {
        if (err || !logPath.trim()) return res.status(503).json({ error: 'Gagal ambil log path: ' + (err?.message || '') })
        exec(`truncate -s 0 ${logPath.trim()}`, { timeout: 5000 }, (err2) => {
            if (err2) return res.status(500).json({ error: 'Gagal hapus log: ' + err2.message })
            res.json({ success: true, message: 'Log berhasil dihapus' })
        })
    })
})

export default router

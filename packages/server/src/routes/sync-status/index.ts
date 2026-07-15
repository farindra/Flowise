import express from 'express'
import { exec } from 'child_process'

const router = express.Router()

function parseLogLine(raw: string): { time: string; message: string; level: string } {
    // Docker --timestamps format: "2026-07-15T03:13:38.123456789Z <message>"
    const match = raw.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.\d+)?Z\s+(.*)$/)
    if (!match) return { time: '', message: raw, level: 'INFO' }
    const message = match[2].replace(/^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2} /, '')
    const level = /fail|error|403|404|refused/i.test(message) ? 'ERROR' : /warn/i.test(message) ? 'WARN' : 'INFO'
    return { time: match[1] + 'Z', message, level }
}

router.get('/logs', (req, res) => {
    const lines = Math.min(parseInt((req.query.lines as string) || '200'), 500)
    exec(`docker logs ob-sync-indexer --timestamps --tail ${lines} 2>&1`, { timeout: 10000 }, (err, stdout) => {
        if (err && !stdout) return res.status(503).json({ error: 'Gagal baca log: ' + err.message })
        const parsed = stdout
            .split('\n')
            .filter(Boolean)
            .map((line) => parseLogLine(line))
            .reverse()
        res.json({ logs: parsed, total: parsed.length })
    })
})

router.get('/status', (req, res) => {
    exec(
        `docker inspect ob-sync-indexer --format '{"running":{{.State.Running}},"startedAt":"{{.State.StartedAt}}","status":"{{.State.Status}}"}'`,
        { timeout: 5000 },
        (err, stdout) => {
            if (err) return res.status(503).json({ error: 'Container tidak ditemukan' })
            try {
                const info = JSON.parse(stdout.trim())
                res.json(info)
            } catch {
                res.status(500).json({ error: 'Parse error' })
            }
        }
    )
})

export default router

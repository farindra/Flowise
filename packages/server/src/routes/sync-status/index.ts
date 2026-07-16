import express from 'express'
import { exec } from 'child_process'
import fs from 'fs'
import path from 'path'

const router = express.Router()

// ── Config paths ──────────────────────────────────────────────────────────────

const SERVICES_DIR = path.resolve(__dirname, '../../../../../services')
const SCHEDULE_FILE = path.join(SERVICES_DIR, 'sync-schedule.json')

const JURNAL_OB_ENV = path.join(SERVICES_DIR, 'go-jurnal-sync/.env')
const JURNAL_SBB_ENV = path.join(SERVICES_DIR, 'go-jurnal-sync/.env.sbb')
const GO_INDEX_ENV = path.join(SERVICES_DIR, 'go-index/.env')
const SYNC_INDEXER_ENV = path.join(SERVICES_DIR, 'trade/sync-indexer/.env')

// ── Service definitions ───────────────────────────────────────────────────────

const SERVICE_DEFS = [
    {
        id: 'ob-sync-indexer',
        label: 'Trade Sync Indexer (OB)',
        description: 'Sinkronisasi katalog trade OB ke Meilisearch (products)',
        type: 'docker',
        index: 'products',
        defaultIntervalMinutes: 30
    },
    {
        id: 'go-index',
        label: 'PrestaShop Indexer',
        description: 'Sinkronisasi produk PrestaShop OB + SBB ke Meilisearch',
        type: 'docker',
        index: 'prime_ob_products, prime_sbb_products',
        defaultIntervalMinutes: 10
    },
    {
        id: 'go-jurnal-sync',
        label: 'Jurnal Sync OB',
        description: 'Sinkronisasi produk & pelanggan Jurnal OB ke Meilisearch',
        type: 'pm2',
        index: 'jurnal_products, customers',
        defaultIntervalMinutes: 720
    },
    {
        id: 'go-jurnal-sync-sbb',
        label: 'Jurnal Sync SBB',
        description: 'Sinkronisasi produk & pelanggan Jurnal SBB ke Meilisearch',
        type: 'pm2',
        index: 'jurnal_sbb_products, sbb_customers',
        defaultIntervalMinutes: 720
    }
]

// ── Schedule config helpers ───────────────────────────────────────────────────

interface ServiceSchedule {
    mode: 'interval' | 'hours'
    intervalMinutes: number
    hours: number[]
    enabled: boolean
}

type ScheduleConfig = Record<string, ServiceSchedule>

function loadSchedule(): ScheduleConfig {
    try {
        if (fs.existsSync(SCHEDULE_FILE)) {
            return JSON.parse(fs.readFileSync(SCHEDULE_FILE, 'utf-8'))
        }
    } catch { /* fall through to default */ }
    return {
        'ob-sync-indexer': { mode: 'interval', intervalMinutes: 30, hours: [], enabled: true },
        'go-index': { mode: 'interval', intervalMinutes: 10, hours: [], enabled: true },
        'go-jurnal-sync': { mode: 'interval', intervalMinutes: 720, hours: [], enabled: true },
        'go-jurnal-sync-sbb': { mode: 'interval', intervalMinutes: 720, hours: [], enabled: true }
    }
}

function saveSchedule(config: ScheduleConfig): void {
    fs.writeFileSync(SCHEDULE_FILE, JSON.stringify(config, null, 2))
}

// ── Trigger command per service ───────────────────────────────────────────────

const TRIGGER_CMDS: Record<string, string> = {
    'ob-sync-indexer': 'docker restart ob-sync-indexer',
    'go-index': 'docker restart go-index',
    'go-jurnal-sync': `curl -s -m 10 -X POST http://127.0.0.1:8084/sync-now -H "X-Internal-Key: ob-jurnal-internal-2026"`,
    'go-jurnal-sync-sbb': `curl -s -m 10 -X POST http://127.0.0.1:8085/sync-now -H "X-Internal-Key: sbb-jurnal-internal-2026"`
}

function triggerService(id: string): void {
    const cmd = TRIGGER_CMDS[id]
    if (!cmd) return
    exec(cmd, { timeout: 30000 }, (err) => {
        if (err) console.error(`[sync-scheduler] failed to trigger ${id}:`, err.message)
        else console.log(`[sync-scheduler] triggered ${id}`)
    })
}

// ── Hours-mode scheduler (WIB) ────────────────────────────────────────────────
// Checks every 30s; fires once per hour per service when minute === 0

const lastFiredHour: Record<string, number> = {}

setInterval(() => {
    const wib = new Date(new Date().toLocaleString('en-US', { timeZone: 'Asia/Jakarta' }))
    if (wib.getMinutes() !== 0) return
    const h = wib.getHours()
    const schedule = loadSchedule()
    for (const [id, sched] of Object.entries(schedule)) {
        if (!sched.enabled || sched.mode !== 'hours') continue
        if (!sched.hours.includes(h)) continue
        if (lastFiredHour[id] === h) continue // already fired this hour
        lastFiredHour[id] = h
        triggerService(id)
    }
}, 30000)

// ── Env file helpers ──────────────────────────────────────────────────────────

function updateEnvKey(filePath: string, key: string, value: string): void {
    let content = fs.readFileSync(filePath, 'utf-8')
    const re = new RegExp(`^(${key}=).*$`, 'm')
    if (re.test(content)) {
        content = content.replace(re, `$1${value}`)
    } else {
        content = content.trimEnd() + `\n${key}=${value}\n`
    }
    fs.writeFileSync(filePath, content)
}

// ── Log parsing ───────────────────────────────────────────────────────────────

function parseDockerLog(raw: string, service: string): { time: string; message: string; level: string; service: string } {
    const match = raw.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.\d+)?Z\s+(.*)$/)
    if (!match) return { time: '', message: raw, level: 'INFO', service }
    const message = match[2].replace(/^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2} /, '')
    const level = /fail|error|403|404|refused/i.test(message) ? 'ERROR' : /warn/i.test(message) ? 'WARN' : 'INFO'
    return { time: match[1] + 'Z', message, level, service }
}

function parsePm2Log(raw: string, service: string): { time: string; message: string; level: string; service: string } | null {
    // PM2 format: "0|go-jurnal-sync  | 2026/07/15 15:32:08 message"
    const match = raw.match(/^\d+\|[^|]+\|\s+(\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2})\s+(.*)$/)
    if (!match) return null
    const message = match[2]
    // Convert "2026/07/15 15:32:08" → ISO (treated as WIB)
    const isoLike = match[1].replace(/\//g, '-').replace(' ', 'T') + '+07:00'
    const level = /fail|error|refused|EOF/i.test(message) ? 'ERROR' : /warn/i.test(message) ? 'WARN' : 'INFO'
    return { time: new Date(isoLike).toISOString(), message, level, service }
}

// ── GET /logs — ob-sync-indexer (legacy, kept for existing UI) ────────────────

router.get('/logs', (req, res) => {
    const lines = Math.min(parseInt((req.query.lines as string) || '200'), 500)
    exec(`docker logs ob-sync-indexer --timestamps --since 168h --tail ${lines} 2>&1`, { timeout: 10000 }, (err, stdout) => {
        if (err && !stdout) return res.status(503).json({ error: 'Gagal baca log: ' + err.message })
        const parsed = stdout
            .split('\n')
            .filter(Boolean)
            .map((line) => parseDockerLog(line, 'ob-sync-indexer'))
            .reverse()
        res.json({ logs: parsed, total: parsed.length })
    })
})

// ── GET /services/:id/logs ────────────────────────────────────────────────────

router.get('/services/:id/logs', (req, res) => {
    const { id } = req.params
    const def = SERVICE_DEFS.find((s) => s.id === id)
    if (!def) return res.status(404).json({ error: 'Service tidak ditemukan' })
    const lines = Math.min(parseInt((req.query.lines as string) || '150'), 500)

    if (def.type === 'docker') {
        exec(`docker logs ${id} --timestamps --since 168h --tail ${lines} 2>&1`, { timeout: 10000 }, (err, stdout) => {
            if (err && !stdout) return res.status(503).json({ error: 'Gagal baca log: ' + err.message })
            const parsed = stdout.split('\n').filter(Boolean).map((l) => parseDockerLog(l, id)).reverse()
            res.json({ logs: parsed, total: parsed.length, service: id, label: def.label })
        })
    } else {
        // PM2 — read from pm2 log file
        exec(`pm2 logs ${id} --lines ${lines} --nostream 2>&1`, { timeout: 10000 }, (err, stdout) => {
            if (err && !stdout) return res.status(503).json({ error: 'Gagal baca log PM2: ' + err.message })
            const parsed = stdout
                .split('\n')
                .filter(Boolean)
                .map((l) => parsePm2Log(l, id))
                .filter(Boolean)
                .reverse() as any[]
            res.json({ logs: parsed, total: parsed.length, service: id, label: def.label })
        })
    }
})

// ── GET /status — ob-sync-indexer ─────────────────────────────────────────────

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

// ── POST /clear-logs — legacy (ob-sync-indexer) ──────────────────────────────

router.post('/clear-logs', (req, res) => {
    exec(`docker inspect --format='{{.LogPath}}' ob-sync-indexer`, { timeout: 5000 }, (err, logPath) => {
        if (err || !logPath.trim()) return res.status(503).json({ error: 'Gagal ambil log path: ' + (err?.message || '') })
        exec(`truncate -s 0 ${logPath.trim()}`, { timeout: 5000 }, (err2) => {
            if (err2) return res.status(500).json({ error: 'Gagal hapus log: ' + err2.message })
            res.json({ success: true, message: 'Log berhasil dihapus' })
        })
    })
})

// ── POST /services/:id/clear-logs ────────────────────────────────────────────

router.post('/services/:id/clear-logs', (req, res) => {
    const { id } = req.params
    const def = SERVICE_DEFS.find((s) => s.id === id)
    if (!def) return res.status(404).json({ error: 'Service tidak ditemukan' })

    if (def.type === 'docker') {
        exec(`docker inspect --format='{{.LogPath}}' ${id}`, { timeout: 5000 }, (err, logPath) => {
            if (err || !logPath.trim()) return res.status(503).json({ error: 'Gagal ambil log path: ' + (err?.message || '') })
            exec(`truncate -s 0 ${logPath.trim()}`, { timeout: 5000 }, (err2) => {
                if (err2) return res.status(500).json({ error: 'Gagal hapus log: ' + err2.message })
                res.json({ success: true, message: 'Log berhasil dihapus' })
            })
        })
    } else {
        // PM2 — flush log files for this process only
        exec(`pm2 flush ${id}`, { timeout: 10000 }, (err) => {
            if (err) return res.status(500).json({ error: 'Gagal hapus log PM2: ' + err.message })
            res.json({ success: true, message: 'Log berhasil dihapus' })
        })
    }
})

// ── GET /services — all services status + schedule ────────────────────────────

router.get('/services', (req, res) => {
    const schedule = loadSchedule()
    const defs = SERVICE_DEFS.map((s) => ({ ...s, schedule: schedule[s.id] || { mode: 'interval', intervalMinutes: s.defaultIntervalMinutes, hours: [], enabled: true } }))

    const tasks: Promise<{ id: string; running: boolean; startedAt?: string }>[] = defs.map((s) => {
        if (s.type === 'docker') {
            return new Promise((resolve) => {
                exec(
                    `docker inspect ${s.id} --format '{"running":{{.State.Running}},"startedAt":"{{.State.StartedAt}}"}'`,
                    { timeout: 5000 },
                    (err, stdout) => {
                        if (err) return resolve({ id: s.id, running: false })
                        try { resolve({ id: s.id, ...JSON.parse(stdout.trim()) }) }
                        catch { resolve({ id: s.id, running: false }) }
                    }
                )
            })
        } else {
            return new Promise((resolve) => {
                exec(`pm2 jlist`, { timeout: 5000 }, (err, stdout) => {
                    if (err) return resolve({ id: s.id, running: false })
                    try {
                        const list = JSON.parse(stdout)
                        const proc = list.find((p: any) => p.name === s.id)
                        if (!proc) return resolve({ id: s.id, running: false })
                        resolve({
                            id: s.id,
                            running: proc.pm2_env?.status === 'online',
                            startedAt: proc.pm2_env?.pm_uptime ? new Date(proc.pm2_env.pm_uptime).toISOString() : undefined
                        })
                    } catch { resolve({ id: s.id, running: false }) }
                })
            })
        }
    })

    const jurnalStatuses: Promise<{ id: string; httpStatus?: any }>[] = [
        { id: 'go-jurnal-sync', port: 8084, key: 'ob-jurnal-internal-2026' },
        { id: 'go-jurnal-sync-sbb', port: 8085, key: 'sbb-jurnal-internal-2026' }
    ].map(({ id, port, key }) =>
        new Promise((resolve) => {
            exec(
                `curl -s -m 3 -X GET http://127.0.0.1:${port}/status -H "X-Internal-Key: ${key}"`,
                { timeout: 5000 },
                (err, stdout) => {
                    if (err || !stdout) return resolve({ id })
                    try { resolve({ id, httpStatus: JSON.parse(stdout) }) }
                    catch { resolve({ id }) }
                }
            )
        })
    )

    Promise.all([Promise.all(tasks), Promise.all(jurnalStatuses)]).then(([statuses, jurnals]) => {
        const statusMap = Object.fromEntries(statuses.map((s) => [s.id, s]))
        const jurnalMap = Object.fromEntries(jurnals.map((j) => [j.id, j.httpStatus]))
        const result = defs.map((s) => ({
            ...s,
            running: statusMap[s.id]?.running ?? false,
            startedAt: statusMap[s.id]?.startedAt,
            httpStatus: jurnalMap[s.id]
        }))
        res.json(result)
    })
})

// ── POST /services/:id/trigger ────────────────────────────────────────────────

router.post('/services/:id/trigger', (req, res) => {
    const { id } = req.params
    const def = SERVICE_DEFS.find((s) => s.id === id)
    if (!def) return res.status(404).json({ error: 'Service tidak ditemukan' })
    const cmd = TRIGGER_CMDS[id]
    if (!cmd) return res.status(400).json({ error: 'Trigger tidak tersedia untuk service ini' })
    exec(cmd, { timeout: 15000 }, (err, stdout) => {
        if (err) return res.status(500).json({ error: 'Gagal trigger: ' + err.message })
        try {
            res.json({ success: true, result: stdout ? JSON.parse(stdout) : { status: 'triggered' } })
        } catch {
            res.json({ success: true, result: { status: 'triggered', output: stdout.trim() } })
        }
    })
})

// ── GET /services/schedule ────────────────────────────────────────────────────

router.get('/services/schedule', (req, res) => {
    res.json(loadSchedule())
})

// ── PUT /services/:id/schedule ────────────────────────────────────────────────

router.put('/services/:id/schedule', (req, res) => {
    const { id } = req.params
    const def = SERVICE_DEFS.find((s) => s.id === id)
    if (!def) return res.status(404).json({ error: 'Service tidak ditemukan' })

    const { mode, intervalMinutes, hours, enabled } = req.body as ServiceSchedule
    if (!mode || (mode === 'interval' && !intervalMinutes) || (mode === 'hours' && !Array.isArray(hours))) {
        return res.status(400).json({ error: 'Body tidak valid' })
    }

    const schedule = loadSchedule()
    schedule[id] = { mode, intervalMinutes: intervalMinutes || def.defaultIntervalMinutes, hours: hours || [], enabled: enabled !== false }
    saveSchedule(schedule)

    const applySchedule = (callback: (err?: string) => void) => {
        const isEnabled = enabled !== false
        // For hours mode + enabled: no process restart needed (scheduler handles it)
        if (mode === 'hours' && isEnabled) return callback()

        const mins = isEnabled ? (intervalMinutes || def.defaultIntervalMinutes) : 525600

        if (id === 'ob-sync-indexer') {
            updateEnvKey(SYNC_INDEXER_ENV, 'CACHE_SYNC_INTERVAL', String(mins * 60000))
            exec('docker restart ob-sync-indexer', { timeout: 30000 }, (err) => callback(err?.message))
        } else if (id === 'go-index') {
            updateEnvKey(GO_INDEX_ENV, 'REINDEX_INTERVAL', String(mins))
            exec('docker restart go-index', { timeout: 30000 }, (err) => callback(err?.message))
        } else if (id === 'go-jurnal-sync') {
            updateEnvKey(JURNAL_OB_ENV, 'SYNC_INTERVAL_HOURS', String(Math.max(1, Math.round(mins / 60))))
            exec('pm2 restart go-jurnal-sync', { timeout: 30000 }, (err) => callback(err?.message))
        } else if (id === 'go-jurnal-sync-sbb') {
            updateEnvKey(JURNAL_SBB_ENV, 'SYNC_INTERVAL_HOURS', String(Math.max(1, Math.round(mins / 60))))
            exec('pm2 restart go-jurnal-sync-sbb', { timeout: 30000 }, (err) => callback(err?.message))
        } else {
            callback()
        }
    }

    applySchedule((err) => {
        if (err) return res.status(500).json({ error: 'Jadwal disimpan tapi gagal restart service: ' + err })
        res.json({ success: true, message: 'Jadwal berhasil disimpan dan diterapkan' })
    })
})

export default router

const http = require('http')

const CRM_KEY = process.env.CRM_INTERNAL_KEY || 'ob-crm-internal-2026'
const WA_KEY = process.env.WA_INTERNAL_KEY || 'ob-wa-internal-2026'
const WA_SESSION = process.env.WA_SESSION_ID || ''
const INTERVAL_MS = parseInt(process.env.INTERVAL_MS || '300000') // 5 menit default

function request(opts, body) {
    return new Promise((resolve, reject) => {
        const req = http.request(opts, (res) => {
            let data = ''
            res.on('data', (c) => (data += c))
            res.on('end', () => {
                try {
                    resolve({ status: res.statusCode, data: JSON.parse(data) })
                } catch {
                    resolve({ status: res.statusCode, data })
                }
            })
        })
        req.on('error', reject)
        if (body) req.write(JSON.stringify(body))
        req.end()
    })
}

async function getActiveWASession() {
    if (WA_SESSION) return WA_SESSION
    const res = await request({
        hostname: '127.0.0.1',
        port: 8082,
        path: '/api/sessions',
        method: 'GET',
        headers: { 'X-Internal-Key': WA_KEY }
    })
    if (!Array.isArray(res.data)) return null
    const active = res.data.find((s) => s.status === 'connected')
    return active ? active.id : null
}

async function sendWA(sessionId, phone, message) {
    const body = JSON.stringify({ phone, message })
    return request(
        {
            hostname: '127.0.0.1',
            port: 8082,
            path: `/api/sessions/${sessionId}/send`,
            method: 'POST',
            headers: {
                'X-Internal-Key': WA_KEY,
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(body)
            }
        },
        { phone, message }
    )
}

async function markSent(notifId) {
    return request({
        hostname: '127.0.0.1',
        port: 8083,
        path: `/api/notifications/${notifId}/sent`,
        method: 'PUT',
        headers: { 'X-Internal-Key': CRM_KEY }
    })
}

async function dispatch() {
    let res
    try {
        res = await request({
            hostname: '127.0.0.1',
            port: 8083,
            path: '/api/notifications/pending',
            method: 'GET',
            headers: { 'X-Internal-Key': CRM_KEY }
        })
    } catch (e) {
        process.stderr.write(`[notif-dispatcher] CRM unavailable: ${e.message}\n`)
        return
    }

    const notifs = Array.isArray(res.data) ? res.data : []
    if (notifs.length === 0) return

    process.stdout.write(`[notif-dispatcher] ${notifs.length} notif(s) pending\n`)

    const sessionId = await getActiveWASession()
    if (!sessionId) {
        process.stderr.write('[notif-dispatcher] No active WA session — skipping WA notifications\n')
    }

    for (const n of notifs) {
        try {
            if (n.channel === 'wa' && sessionId) {
                const r = await sendWA(sessionId, n.recipient_phone, n.message)
                if (r.status < 300) {
                    await markSent(n.id)
                    process.stdout.write(`[notif-dispatcher] Sent WA to ${n.recipient_phone} (${n.type})\n`)
                } else {
                    process.stderr.write(`[notif-dispatcher] WA send failed ${r.status}: ${JSON.stringify(r.data)}\n`)
                }
            } else if (n.channel !== 'wa') {
                // channel lain belum diimplementasi — skip tapi tetap mark sent
                process.stdout.write(`[notif-dispatcher] Skip channel=${n.channel} id=${n.id}\n`)
                await markSent(n.id)
            }
        } catch (e) {
            process.stderr.write(`[notif-dispatcher] Error sending notif ${n.id}: ${e.message}\n`)
        }
    }
}

async function run() {
    process.stdout.write(`[notif-dispatcher] Starting, interval=${INTERVAL_MS}ms, wa_session=${WA_SESSION || 'auto'}\n`)
    dispatch().catch((e) => process.stderr.write(`[notif-dispatcher] ${e.message}\n`))
    setInterval(() => {
        dispatch().catch((e) => process.stderr.write(`[notif-dispatcher] ${e.message}\n`))
    }, INTERVAL_MS)
}

run()

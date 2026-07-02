#!/usr/bin/env node
/**
 * Sensor simulator — kirim data sensor sintetis ke go-iot setiap INTERVAL detik
 * Usage: node simulator.js [--interval 30] [--scenario normal|dry|hot|night]
 */

const http = require('http')

const IOT_KEY = process.env.IOT_INTERNAL_KEY || 'ob-iot-internal-2026'
const INTERVAL = parseInt(process.env.INTERVAL || '30') * 1000
const SCENARIO = process.env.SCENARIO || 'normal'

const ZONES = ['zona-a', 'zona-b', 'zona-c', 'taman-utama', 'parkir']

function rand(min, max) {
    return +(Math.random() * (max - min) + min).toFixed(1)
}

function generateReadings(scenario) {
    const now = new Date().toISOString()
    const readings = []

    for (const zoneId of ZONES) {
        let soilHumidity, temperature, airHumidity

        switch (scenario) {
            case 'dry':
                soilHumidity = rand(25, 39) // di bawah threshold 40%
                temperature = rand(30, 34)
                airHumidity = rand(45, 60)
                break
            case 'hot':
                soilHumidity = rand(42, 65)
                temperature = rand(36, 42) // di atas threshold 35°C
                airHumidity = rand(38, 48) // di bawah threshold 50%
                break
            case 'night':
                soilHumidity = rand(55, 75)
                temperature = rand(22, 26)
                airHumidity = rand(70, 90)
                break
            default: // normal
                soilHumidity = rand(50, 75)
                temperature = rand(27, 33)
                airHumidity = rand(55, 75)
        }

        readings.push({ zone_id: zoneId, sensor_type: 'soil_humidity', value: soilHumidity, unit: '%', recorded_at: now })
        readings.push({ zone_id: zoneId, sensor_type: 'temperature', value: temperature, unit: '°C', recorded_at: now })
        readings.push({ zone_id: zoneId, sensor_type: 'air_humidity', value: airHumidity, unit: '%', recorded_at: now })

        // Gerakan malam hari di area parkir (simulasi CCTV)
        if (zoneId === 'parkir' && scenario === 'night' && Math.random() < 0.1) {
            readings.push({ zone_id: 'parkir', sensor_type: 'motion', value: 1, unit: '', recorded_at: now })
        }
    }

    return readings
}

function sendReadings(readings) {
    return new Promise((resolve, reject) => {
        const body = JSON.stringify(readings)
        const req = http.request(
            {
                hostname: '127.0.0.1',
                port: 8084,
                path: '/api/readings',
                method: 'POST',
                headers: { 'X-Internal-Key': IOT_KEY, 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body) }
            },
            (res) => {
                let data = ''
                res.on('data', (c) => (data += c))
                res.on('end', () => resolve({ status: res.statusCode, data: JSON.parse(data) }))
            }
        )
        req.on('error', reject)
        req.write(body)
        req.end()
    })
}

async function tick() {
    const readings = generateReadings(SCENARIO)
    try {
        const res = await sendReadings(readings)
        const ts = new Date().toLocaleTimeString('id-ID')
        process.stdout.write(`[${ts}] [simulator] Sent ${readings.length} readings (scenario=${SCENARIO}) → saved=${res.data.saved}\n`)
    } catch (e) {
        process.stderr.write(`[simulator] Error: ${e.message}\n`)
    }
}

process.stdout.write(`[simulator] Starting — scenario=${SCENARIO}, interval=${INTERVAL / 1000}s\n`)
tick()
setInterval(tick, INTERVAL)

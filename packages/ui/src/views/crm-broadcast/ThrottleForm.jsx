import PropTypes from 'prop-types'
import { Box, Divider, Grid, Stack, TextField, Typography } from '@mui/material'

// Grouped so the composer reads as "how fast / how often / when / what if it fails"
// rather than a flat wall of numbers.
const GROUPS = [
    {
        title: 'Jeda antar pesan',
        fields: [
            { key: 'min_delay_sec', label: 'Jeda minimum (detik)', type: 'number' },
            { key: 'max_delay_sec', label: 'Jeda maksimum (detik)', type: 'number' }
        ]
    },
    {
        title: 'Batch',
        fields: [
            { key: 'batch_size', label: 'Pesan per batch', type: 'number' },
            { key: 'batch_pause_sec', label: 'Jeda antar batch (detik)', type: 'number' }
        ]
    },
    {
        title: 'Batas',
        fields: [
            { key: 'daily_cap', label: 'Batas harian per pengirim', type: 'number' },
            { key: 'cooldown_days', label: 'Cooldown per nomor (hari)', type: 'number' },
            { key: 'warmup_first_n', label: 'Pemanasan N pesan pertama', type: 'number' }
        ]
    },
    {
        title: 'Jam kerja',
        fields: [
            { key: 'hours_start', label: 'Mulai (HH:MM)', type: 'text' },
            { key: 'hours_end', label: 'Selesai (HH:MM)', type: 'text' }
        ]
    },
    {
        title: 'Percobaan ulang',
        fields: [
            { key: 'max_attempts', label: 'Maks percobaan', type: 'number' },
            { key: 'retry_backoff_sec', label: 'Jeda retry awal (detik)', type: 'number' },
            { key: 'retry_backoff_max_sec', label: 'Jeda retry maks (detik)', type: 'number' }
        ]
    }
]

const ThrottleForm = ({ value, onChange, defaults, limits }) => {
    const set = (key, raw, type) => {
        const next = { ...value }
        if (raw === '' || raw === null) {
            delete next[key] // cleared field falls back to the global default
        } else {
            next[key] = type === 'number' ? Number(raw) : raw
        }
        onChange(next)
    }

    return (
        <Stack spacing={2}>
            <Typography variant='caption' color='text.secondary'>
                Kosongkan field untuk memakai nilai default server. Nilai default ditampilkan di bawah tiap kolom.
            </Typography>

            {GROUPS.map((g) => (
                <Box key={g.title}>
                    <Typography variant='subtitle2' sx={{ mb: 1 }}>
                        {g.title}
                    </Typography>
                    <Grid container spacing={1.5}>
                        {g.fields.map((f) => (
                            <Grid item xs={12} sm={4} key={f.key}>
                                <TextField
                                    fullWidth
                                    size='small'
                                    type={f.type}
                                    label={f.label}
                                    value={value[f.key] ?? ''}
                                    onChange={(e) => set(f.key, e.target.value, f.type)}
                                    placeholder={String(defaults?.[f.key] ?? '')}
                                    helperText={`default: ${defaults?.[f.key] ?? '-'}`}
                                />
                            </Grid>
                        ))}
                    </Grid>
                    <Divider sx={{ mt: 2 }} />
                </Box>
            ))}

            {limits && (
                <Typography variant='caption' color='text.secondary'>
                    Batas server: jeda minimum tidak boleh di bawah {limits.min_delay_floor_sec} detik, batas harian maksimal{' '}
                    {limits.hard_daily_cap}, maksimal {limits.max_recipients} penerima per campaign.
                </Typography>
            )}
        </Stack>
    )
}

ThrottleForm.propTypes = {
    value: PropTypes.object.isRequired,
    onChange: PropTypes.func.isRequired,
    defaults: PropTypes.object,
    limits: PropTypes.object
}

export default ThrottleForm

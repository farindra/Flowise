import { useEffect, useState, useRef } from 'react'
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Alert,
    Box,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Divider,
    IconButton,
    MenuItem,
    Select,
    Stack,
    TextField,
    Tooltip,
    Typography
} from '@mui/material'
import {
    IconChevronDown,
    IconEdit,
    IconTrash,
    IconPlus,
    IconFileTypePdf,
    IconDeviceFloppy,
    IconX,
    IconHelp
} from '@tabler/icons-react'
import MainCard from '@/ui-component/cards/MainCard'
import apiClient from '@/api/client'

const CATEGORIES = ['WhatsApp Bot', 'Upload Data', 'Chatflow & AI', 'Sistem & Maintenance', 'Lainnya']

function genId() {
    return Date.now().toString(36) + Math.random().toString(36).slice(2)
}

export default function FAQ() {
    const [items, setItems] = useState([])
    const [loading, setLoading] = useState(true)
    const [saving, setSaving] = useState(false)
    const [error, setError] = useState('')
    const [success, setSuccess] = useState('')
    const [editMode, setEditMode] = useState(false)
    const [dialog, setDialog] = useState(null) // null | { mode: 'add'|'edit', item }
    const [expanded, setExpanded] = useState(false)
    const printRef = useRef()

    useEffect(() => {
        apiClient.get('/faq').then((r) => setItems(r.data)).catch(() => setError('Gagal memuat FAQ')).finally(() => setLoading(false))
    }, [])

    const save = async (newItems) => {
        setSaving(true)
        try {
            await apiClient.post('/faq', newItems)
            setItems(newItems)
            setSuccess('FAQ berhasil disimpan')
            setTimeout(() => setSuccess(''), 3000)
        } catch {
            setError('Gagal menyimpan FAQ')
        } finally {
            setSaving(false)
        }
    }

    const handleDelete = (id) => {
        if (!window.confirm('Hapus item FAQ ini?')) return
        save(items.filter((i) => i.id !== id))
    }

    const handleDialogSave = (formData) => {
        let newItems
        if (dialog.mode === 'add') {
            newItems = [...items, { ...formData, id: genId() }]
        } else {
            newItems = items.map((i) => (i.id === formData.id ? formData : i))
        }
        save(newItems)
        setDialog(null)
    }

    const handleExportPdf = () => {
        const printContent = document.getElementById('faq-print-area')
        const win = window.open('', '_blank')
        win.document.write(`
            <html>
            <head>
                <title>FAQ - Ocean Bearing Agentic</title>
                <style>
                    body { font-family: Arial, sans-serif; padding: 32px; color: #111; }
                    h1 { font-size: 22px; margin-bottom: 4px; }
                    .subtitle { color: #666; font-size: 13px; margin-bottom: 32px; }
                    .category { font-size: 13px; font-weight: bold; color: #1976d2; text-transform: uppercase;
                        letter-spacing: 1px; margin: 24px 0 8px; border-bottom: 2px solid #1976d2; padding-bottom: 4px; }
                    .item { margin-bottom: 16px; }
                    .question { font-weight: bold; font-size: 14px; margin-bottom: 4px; }
                    .answer { font-size: 13px; color: #333; line-height: 1.6; }
                    @media print { body { padding: 16px; } }
                </style>
            </head>
            <body>
                <h1>FAQ — Ocean Bearing Agentic</h1>
                <div class="subtitle">Panduan penggunaan sistem Agentic OB | Dicetak: ${new Date().toLocaleDateString('id-ID', { dateStyle: 'long' })}</div>
                ${printContent.innerHTML}
            </body>
            </html>
        `)
        win.document.close()
        setTimeout(() => { win.focus(); win.print() }, 300)
    }

    const grouped = CATEGORIES.reduce((acc, cat) => {
        const catItems = items.filter((i) => i.category === cat)
        if (catItems.length) acc[cat] = catItems
        return acc
    }, {})
    // Add uncategorized
    const otherItems = items.filter((i) => !CATEGORIES.includes(i.category))
    if (otherItems.length) grouped['Lainnya'] = [...(grouped['Lainnya'] || []), ...otherItems]

    return (
        <MainCard>
            {/* Header */}
            <Stack direction='row' alignItems='center' justifyContent='space-between' mb={3} flexWrap='wrap' gap={1}>
                <Stack direction='row' alignItems='center' gap={1}>
                    <IconHelp size={28} />
                    <Box>
                        <Typography variant='h3'>FAQ</Typography>
                        <Typography variant='caption' color='text.secondary'>Panduan Penggunaan Agentic Ocean Bearing</Typography>
                    </Box>
                </Stack>
                <Stack direction='row' gap={1} flexWrap='wrap'>
                    {editMode && (
                        <Button variant='contained' size='small' startIcon={<IconPlus size={16} />}
                            onClick={() => setDialog({ mode: 'add', item: { category: CATEGORIES[0], question: '', answer: '' } })}>
                            Tambah
                        </Button>
                    )}
                    <Button variant='outlined' size='small'
                        startIcon={editMode ? <IconX size={16} /> : <IconEdit size={16} />}
                        color={editMode ? 'error' : 'primary'}
                        onClick={() => setEditMode(!editMode)}>
                        {editMode ? 'Selesai Edit' : 'Edit FAQ'}
                    </Button>
                    <Button variant='outlined' size='small' startIcon={<IconFileTypePdf size={16} />}
                        onClick={handleExportPdf} color='error'>
                        Export PDF
                    </Button>
                </Stack>
            </Stack>

            {error && <Alert severity='error' sx={{ mb: 2 }} onClose={() => setError('')}>{error}</Alert>}
            {success && <Alert severity='success' sx={{ mb: 2 }} onClose={() => setSuccess('')}>{success}</Alert>}
            {saving && <Alert severity='info' sx={{ mb: 2 }}>Menyimpan...</Alert>}

            {loading ? (
                <Typography color='text.secondary'>Memuat FAQ...</Typography>
            ) : (
                <Box>
                    {/* Hidden print area */}
                    <Box id='faq-print-area' sx={{ display: 'none' }}>
                        {Object.entries(grouped).map(([cat, catItems]) => (
                            <div key={cat}>
                                <div className='category'>{cat}</div>
                                {catItems.map((item) => (
                                    <div key={item.id} className='item'>
                                        <div className='question'>Q: {item.question}</div>
                                        <div className='answer'>{item.answer}</div>
                                    </div>
                                ))}
                            </div>
                        ))}
                    </Box>

                    {/* FAQ Accordion */}
                    {Object.entries(grouped).map(([cat, catItems]) => (
                        <Box key={cat} mb={3}>
                            <Stack direction='row' alignItems='center' gap={1} mb={1}>
                                <Chip label={cat} size='small' color='primary' variant='outlined' />
                                <Typography variant='caption' color='text.secondary'>{catItems.length} item</Typography>
                            </Stack>
                            {catItems.map((item) => (
                                <Accordion
                                    key={item.id}
                                    expanded={expanded === item.id}
                                    onChange={(_, open) => setExpanded(open ? item.id : false)}
                                    sx={{ mb: 0.5, '&:before': { display: 'none' }, border: '1px solid', borderColor: 'divider' }}
                                    elevation={0}
                                >
                                    <AccordionSummary expandIcon={<IconChevronDown size={18} />}>
                                        <Stack direction='row' alignItems='center' justifyContent='space-between' width='100%' pr={1}>
                                            <Typography variant='body1' fontWeight={500}>{item.question}</Typography>
                                            {editMode && (
                                                <Stack direction='row' gap={0.5} onClick={(e) => e.stopPropagation()}>
                                                    <Tooltip title='Edit'>
                                                        <IconButton size='small' onClick={() => setDialog({ mode: 'edit', item: { ...item } })}>
                                                            <IconEdit size={15} />
                                                        </IconButton>
                                                    </Tooltip>
                                                    <Tooltip title='Hapus'>
                                                        <IconButton size='small' color='error' onClick={() => handleDelete(item.id)}>
                                                            <IconTrash size={15} />
                                                        </IconButton>
                                                    </Tooltip>
                                                </Stack>
                                            )}
                                        </Stack>
                                    </AccordionSummary>
                                    <AccordionDetails>
                                        <Typography variant='body2' color='text.secondary' sx={{ whiteSpace: 'pre-line', lineHeight: 1.7 }}>
                                            {item.answer}
                                        </Typography>
                                    </AccordionDetails>
                                </Accordion>
                            ))}
                        </Box>
                    ))}

                    {items.length === 0 && (
                        <Box textAlign='center' py={6}>
                            <Typography color='text.secondary'>Belum ada FAQ. Klik "Edit FAQ" → "Tambah" untuk mulai.</Typography>
                        </Box>
                    )}
                </Box>
            )}

            {/* Add/Edit Dialog */}
            {dialog && (
                <FAQDialog
                    mode={dialog.mode}
                    item={dialog.item}
                    categories={CATEGORIES}
                    onSave={handleDialogSave}
                    onClose={() => setDialog(null)}
                />
            )}
        </MainCard>
    )
}

function FAQDialog({ mode, item, categories, onSave, onClose }) {
    const [form, setForm] = useState({ ...item })

    const set = (key, val) => setForm((f) => ({ ...f, [key]: val }))

    return (
        <Dialog open fullWidth maxWidth='sm' onClose={onClose}>
            <DialogTitle>{mode === 'add' ? 'Tambah FAQ' : 'Edit FAQ'}</DialogTitle>
            <DialogContent>
                <Stack gap={2} mt={1}>
                    <Select size='small' value={form.category} onChange={(e) => set('category', e.target.value)} fullWidth>
                        {categories.map((c) => <MenuItem key={c} value={c}>{c}</MenuItem>)}
                    </Select>
                    <TextField
                        label='Pertanyaan'
                        value={form.question}
                        onChange={(e) => set('question', e.target.value)}
                        fullWidth size='small' multiline rows={2}
                    />
                    <TextField
                        label='Jawaban'
                        value={form.answer}
                        onChange={(e) => set('answer', e.target.value)}
                        fullWidth size='small' multiline rows={5}
                        helperText='Bisa gunakan baris baru untuk poin-poin'
                    />
                </Stack>
            </DialogContent>
            <DialogActions>
                <Button onClick={onClose} color='inherit'>Batal</Button>
                <Button
                    variant='contained'
                    startIcon={<IconDeviceFloppy size={16} />}
                    onClick={() => onSave(form)}
                    disabled={!form.question.trim() || !form.answer.trim()}
                >
                    Simpan
                </Button>
            </DialogActions>
        </Dialog>
    )
}

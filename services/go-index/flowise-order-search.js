const axios = require('axios');

const MEILI_URL = 'http://127.0.0.1:7700';
const MEILI_KEY = '61434e3b7b15c9c890dd0785d3bb9c555d3d831b217249f31aeb547532ac721b';
const INDEX     = 'prime_ob_orders'; // prime_ob_orders atau prime_sbb_orders

const identifier = $order_identifier.trim();

const statusMap = {
    'Menunggu pembayaran via Transfer Bank': 'Menunggu pembayaran via Transfer Bank',
    'Pembayaran berhasil dikonfirmasi'      : 'Pembayaran berhasil dikonfirmasi',
    'Pesanan sedang dipersiapkan'           : 'Pesanan sedang dipersiapkan',
    'Paket sudah dikirim (Dalam Perjalanan)': 'Paket sudah dikirim (Dalam Perjalanan)',
    'Paket sudah diterima (Selesai)'        : 'Paket sudah diterima (Selesai)',
    'Pesanan dibatalkan'                    : 'Pesanan dibatalkan',
    'Pengembalian dana (Refunded)'          : 'Pengembalian dana (Refunded)',
    'Menunggu pembayaran via COD'           : 'Menunggu pembayaran via COD'
};

try {
    // Cari by ID numerik atau referensi teks
    const isNumeric = /^\d+$/.test(identifier);

    const body = {
        q: identifier,
        limit: 5,
        attributesToRetrieve: ['id_order', 'reference', 'customer_name', 'email', 'total_paid', 'status', 'date_add', 'items']
    };

    if (isNumeric) {
        body.filter = `id_order = ${identifier}`;
    }
    // Kalau referensi teks (misal: ABCDE), full-text search sudah cukup

    const response = await axios.post(
        `${MEILI_URL}/indexes/${INDEX}/search`,
        body,
        { headers: { 'Authorization': `Bearer ${MEILI_KEY}` } }
    );

    const hits = response.data.hits;

    if (!hits || hits.length === 0) {
        return `Pesanan dengan referensi/ID "${identifier}" tidak ditemukan.`;
    }

    const order = hits[0];

    const result = {
        id_pesanan      : order.id_order,
        referensi       : order.reference,
        nama_pelanggan  : order.customer_name,
        total_bayar     : `Rp ${Math.round(order.total_paid).toLocaleString('id-ID')}`,
        tanggal_pesan   : order.date_add,
        status_pesanan  : order.status,
        item_yang_dibeli: order.items ? order.items.split(' | ') : []
    };

    return JSON.stringify(result, null, 2);

} catch (error) {
    return `Gagal mengambil data pesanan: ${error.message}`;
}

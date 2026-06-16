const axios = require('axios');

const MEILI_URL  = 'http://127.0.0.1:7700';
const MEILI_KEY  = '61434e3b7b15c9c890dd0785d3bb9c555d3d831b217249f31aeb547532ac721b';
const STORE_URL  = 'https://prime.oceanbearings.co.id';
const INDEX      = 'prime_ob_products';
const WA_NUMBER  = '6281234567890'; // Ganti dengan nomor WA CS, contoh: 6281234567890
const WA_TEXT    = 'Halo, saya ingin melakukan Purchase Order (PO) manual.';

// Input dari AI agent — semua pakai try/catch supaya tidak crash jika belum dikonfigurasi
let searchType = 'name';
try { searchType = ($search_type || 'name').trim(); } catch(e) { searchType = 'name'; }

let query = '';
try { query = ($query || '').trim(); } catch(e) { query = ''; }

let fallbackQuery = '';
try { fallbackQuery = ($fallback_query || '').trim(); } catch(e) { fallbackQuery = ''; }

// encodeURIComponent tidak encode ( dan ) — encode manual supaya tidak rusak di markdown link
const WA_LINK = `https://wa.me/${WA_NUMBER}?text=${encodeURIComponent(WA_TEXT).replace(/\(/g,'%28').replace(/\)/g,'%29')}`;

if (!query) return JSON.stringify({ error: 'query kosong', found: 0, products: [], po_manual: { whatsapp_link: WA_LINK } });

async function searchProducts(q, type, extraFilter) {
    const body = {
        limit: 10,
        attributesToRetrieve: ['id_product', 'reference', 'name', 'price', 'stock', 'categories', 'link_rewrite']
    };

    if (type === 'category') {
        body.q = '';
        body.filter = [`categories = "${q}"`, 'active = true'];
    } else if (type === 'reference') {
        body.q = q;
        body.filter = ['active = true'];
    } else {
        body.q = q;
        body.filter = extraFilter ? [extraFilter, 'active = true'] : ['active = true'];
    }

    const res = await axios.post(
        `${MEILI_URL}/indexes/${INDEX}/search`,
        body,
        { headers: { 'Authorization': `Bearer ${MEILI_KEY}` } }
    );
    return res.data.hits || [];
}

function formatProducts(hits) {
    return hits.map(p => ({
        id: p.id_product,
        reference: p.reference,
        name: p.name,
        price: p.price > 0 ? `Rp ${Math.round(p.price).toLocaleString('id-ID')}` : 'Hubungi CS',
        stock: p.stock > 0 ? p.stock : 'Habis / Indent',
        categories: p.categories,
        link: p.link_rewrite
            ? `${STORE_URL}/${p.id_product}-${p.link_rewrite}.html`
            : STORE_URL
    }));
}

try {
    // Pencarian utama
    let hits = await searchProducts(query, searchType);

    // Kalau hasil utama kosong dan ada fallback query → cari pakai ukuran bearing
    let usedFallback = false;
    if (hits.length === 0 && fallbackQuery) {
        hits = await searchProducts(fallbackQuery, 'name');
        usedFallback = true;
    }

    const result = {
        found: hits.length,
        searched_by: usedFallback ? `fallback: "${fallbackQuery}"` : `"${query}"`,
        products: hits.length > 0 ? formatProducts(hits) : [],
        po_manual: {
            info: 'Tidak menemukan produk yang dicari? Hubungi tim admin untuk PO manual.',
            whatsapp_link: WA_LINK
        }
    };

    return JSON.stringify(result, null, 2);

} catch (error) {
    return JSON.stringify({
        found: 0,
        products: [],
        error: error.message,
        po_manual: {
            info: 'Terjadi kesalahan pencarian. Hubungi tim admin untuk bantuan.',
            whatsapp_link: WA_LINK
        }
    });
}

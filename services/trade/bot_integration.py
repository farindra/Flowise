"""
\U0001f916 BOT INTEGRATION API ROUTES
Provides optimized endpoints for external bot integrations.
"""

import re
from typing import List, Dict, Any, Optional
from fastapi import APIRouter, Depends, HTTPException, Header, status
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, and_, text
from decimal import Decimal

from src.api.dependencies import get_db
from src.models.database import Products, Brands
from src.config.settings import settings
from src.config.logging import get_logger

logger = get_logger(__name__)
router = APIRouter(prefix="/bot-integration", tags=["Bot Integration"])

# Security Dependency
async def verify_bot_api_key(x_api_key: str = Header(...)):
    """Verify that the provided API key matches the configured BOT_API_KEY."""
    if x_api_key != settings.BOT_API_KEY:
        logger.warning("Unauthorized bot API access attempt")
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid API Key"
        )
    return x_api_key

def format_idr(value: Optional[Decimal]) -> str:
    """Format decimal value to IDR currency string."""
    if value is None:
        return "Rp. 0,00"
    try:
        # Format: Rp. 45.000,00
        # Use simple formatting for speed, could be more robust with locale
        formatted = "{:,.2f}".format(float(value)).replace(",", "X").replace(".", ",").replace("X", ".")
        return f"Rp. {formatted}"
    except Exception:
        return "Rp. 0,00"

def extract_brand_fallback(code: str, name: str) -> str:
    """Fallback logic to extract brand from code or name if not explicitly set."""
    brands = ["NSK", "KOYO", "INA", "FAG", "SKF", "TIMKEN", "NTN", "NACHI", "IKO", "ASAHI", "FYH"]
    text = (f"{code} {name}").upper()
    for brand in brands:
        if brand in text:
            return brand
    return "UNKNOWN"

@router.get("/products", response_model=List[Dict[str, Any]])
async def get_bot_products(
    db: AsyncSession = Depends(get_db),
    api_key: str = Depends(verify_bot_api_key)
):
    """
    Fetch all active products with 100% identical mapping to the Bot system.
    """
    try:
        from datetime import datetime
        
        # Optimized query
        stmt = (
            select(
                func.min(Products.id).label("id"),
                Products.product_code,
                func.max(Products.product_name).label("product_name"),
                func.jsonb_agg(Products.specifications).label("all_specs"),
                func.sum(Products.stock_quantity).label("total_stock"),
                func.max(Products.price_customer).label("price_customer"),
                func.max(Products.price_normal).label("price_normal"),
                func.max(Products.price_cash).label("price_cash"),
                func.bool_or(Products.is_active).label("is_active"),
                func.bool_or(Products.is_available).label("is_available"),
                func.max(Products.updated_at).label("updated_at")
            )
            .where(Products.is_active == True)
            .group_by(Products.product_code)
        )
        
        result = await db.execute(stmt)
        rows = result.all()
        
        products_payload = []
        for row in rows:
            code = str(row.product_code)
            
            # 1. Brand extraction: part after the last dot
            parts = code.split('.')
            brand = parts[-1] if len(parts) > 1 else "UNKNOWN"
            
            # 2. Identitas: nama and keterangan
            name = row.product_name
            all_specs = row.all_specs or []
            specs = all_specs[0] if all_specs and all_specs[0] else {}
            keterangan = specs.get('keterangan', '')
            
            # 3. Pricing logic: Pull directly from DB (already calculated +10% during import)
            base_price = Decimal(str(row.price_customer or 0))
            normal_price = Decimal(str(row.price_normal or 0))
            cash_price = Decimal(str(row.price_cash or 0))
            
            # Numeric values for _hargaNum
            p_customer_num = float(base_price)
            p_normal_num = float(normal_price)
            p_cash_num = float(cash_price)
            
            # 4. Formatting
            stok_total = int(row.total_stock or 0)
            
            # lastUpdated formatting with Z suffix
            updated_at_str = row.updated_at.isoformat() if row.updated_at else datetime.utcnow().isoformat()
            if not updated_at_str.endswith('Z'):
                updated_at_str += 'Z'

            products_payload.append({
                "id": row.id,
                "kode": code,
                "nama": name,
                "keterangan": keterangan,
                "stok": stok_total,
                "brand": brand,
                "hargaNormal": format_idr(normal_price),
                "hargaCustomer": format_idr(base_price),
                "hargaNonCustomer": format_idr(normal_price),
                "hargaCash": format_idr(cash_price),
                "_hargaNum": {
                    "customer": p_customer_num,
                    "normal": int(p_normal_num),
                    "cash": int(p_cash_num),
                    "nonCustomer": int(p_normal_num)
                },
                "active": bool(row.is_active),
                "available": stok_total > 0,
                "lastUpdated": updated_at_str
            })
            
        logger.info(f"Bot integration: returning {len(products_payload)} products with 100% matched logic")
        return products_payload
        
    except Exception as e:
        logger.error(f"Error in bot products integration: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to fetch products for bot integration"
        )


# ─── Owner Assistant Endpoints ────────────────────────────────────────────────

@router.get("/owner/low-stock")
async def owner_low_stock(
    limit: int = 100,
    brand: Optional[str] = None,
    db: AsyncSession = Depends(get_db),
    api_key: str = Depends(verify_bot_api_key)
):
    """Stok kosong / kritis untuk Owner Assistant."""
    try:
        where = [
            "p.is_active = true",
            "p.stock_quantity <= COALESCE(p.min_stock_threshold, 10)"
        ]
        params: dict = {"limit": min(limit, 500)}
        if brand:
            where.append("b.brand_name ILIKE :brand")
            params["brand"] = f"%{brand}%"

        sql = text(f"""
            SELECT p.product_code, p.product_name,
                   COALESCE(b.brand_name, '') AS brand,
                   p.stock_quantity, p.min_stock_threshold,
                   p.product_category,
                   p.price_normal, p.price_customer
            FROM products p
            LEFT JOIN brands b ON p.brand_id = b.id
            WHERE {' AND '.join(where)}
            ORDER BY p.stock_quantity ASC, p.product_code
            LIMIT :limit
        """)
        res = await db.execute(sql, params)
        rows = [dict(r._mapping) for r in res.fetchall()]
        return {"total": len(rows), "items": rows}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/owner/trade-summary")
async def owner_trade_summary(
    days: int = 30,
    db: AsyncSession = Depends(get_db),
    api_key: str = Depends(verify_bot_api_key)
):
    """Ringkasan data perdagangan N hari terakhir."""
    try:
        sql = text("""
            SELECT
                COUNT(*) AS total_transaksi,
                COUNT(DISTINCT supplier_name) AS jumlah_supplier,
                SUM(total_amount_idr) AS total_nilai_idr,
                MIN(trade_date) AS dari_tanggal,
                MAX(trade_date) AS sampai_tanggal
            FROM trade_data
            WHERE trade_date >= CURRENT_DATE - :days
        """)
        res = await db.execute(sql, {"days": days})
        row = dict(res.fetchone()._mapping)

        top_sql = text("""
            SELECT supplier_name, COUNT(*) AS transaksi,
                   SUM(total_amount_idr) AS nilai_idr
            FROM trade_data
            WHERE trade_date >= CURRENT_DATE - :days
              AND supplier_name IS NOT NULL
            GROUP BY supplier_name
            ORDER BY nilai_idr DESC NULLS LAST
            LIMIT 10
        """)
        top_res = await db.execute(top_sql, {"days": days})
        top_suppliers = [dict(r._mapping) for r in top_res.fetchall()]

        return {"summary": row, "top_suppliers": top_suppliers}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/owner/supplier-prices")
async def owner_supplier_prices(
    brand: Optional[str] = None,
    product_code: Optional[str] = None,
    limit: int = 50,
    db: AsyncSession = Depends(get_db),
    api_key: str = Depends(verify_bot_api_key)
):
    """Harga penawaran supplier terbaru dari upload price comparison."""
    try:
        where = ["r.is_active = true"]
        params: dict = {"limit": min(limit, 200)}
        if brand:
            where.append("r.brand ILIKE :brand")
            params["brand"] = f"%{brand}%"
        if product_code:
            where.append("r.product_code ILIKE :product_code")
            params["product_code"] = f"%{product_code}%"

        sql = text(f"""
            SELECT r.product_code, r.product_name, r.brand,
                   r.supplier_price, r.supplier_currency,
                   r.price_idr, r.price_status,
                   u.original_filename AS sumber_file,
                   u.upload_date
            FROM supplier_price_upload_rows r
            JOIN supplier_price_uploads u ON r.upload_id = u.id
            WHERE {' AND '.join(where)}
            ORDER BY u.upload_date DESC, r.product_code
            LIMIT :limit
        """)
        res = await db.execute(sql, params)
        rows = [dict(r._mapping) for r in res.fetchall()]
        return {"total": len(rows), "items": rows}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/owner/purchase-orders")
async def owner_purchase_orders(
    limit: int = 20,
    status: Optional[str] = None,
    db: AsyncSession = Depends(get_db),
    api_key: str = Depends(verify_bot_api_key)
):
    """Daftar Purchase Order terbaru."""
    try:
        where = ["po.is_active = true"]
        params: dict = {"limit": min(limit, 100)}
        if status:
            where.append("po.status = :status")
            params["status"] = status

        sql = text(f"""
            SELECT po.po_number, po.supplier_name, po.order_date,
                   po.expected_arrival_date, po.status,
                   COUNT(poi.id) AS jumlah_item,
                   po.created_at
            FROM purchase_orders po
            LEFT JOIN purchase_order_items poi ON poi.purchase_order_id = po.id
            WHERE {' AND '.join(where)}
            GROUP BY po.id
            ORDER BY po.created_at DESC
            LIMIT :limit
        """)
        res = await db.execute(sql, params)
        rows = [dict(r._mapping) for r in res.fetchall()]
        return {"total": len(rows), "purchase_orders": rows}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

# ─── Owner: Upload Supplier Offer ─────────────────────────────────────────────

import os
import tempfile
from uuid import uuid4
from fastapi import UploadFile, File, Form, BackgroundTasks

BOT_USER_ID = 2  # admin user for bot-submitted uploads

async def _run_offer_processor(file_path: str, upload_id: str, original_filename: str,
                                supplier_name: str | None, currency: str):
    """Run OfferProcessorService in background and clean up temp file."""
    from src.services.offer_processor import OfferProcessorService
    processor = OfferProcessorService()
    try:
        await processor.process_upload(
            upload_id=upload_id,
            file_path=file_path,
            user_id=BOT_USER_ID,
            original_filename=original_filename,
            supplier_name=supplier_name,
            default_currency=currency,
        )
    finally:
        if os.path.exists(file_path):
            os.remove(file_path)

@router.post("/owner/upload-supplier-offer")
async def bot_upload_supplier_offer(
    background_tasks: BackgroundTasks,
    file: UploadFile = File(...),
    supplier_name: Optional[str] = Form(None),
    currency: str = Form("USD"),
    db: AsyncSession = Depends(get_db),
    _: str = Depends(verify_bot_api_key),
):
    """
    Upload supplier offer Excel from bot (WA/Telegram).
    Auto-maps columns, normalizes currency, triggers background price analysis.
    Accepts .xlsx or .xls — dynamic header detection handles any column layout.
    """
    fname = file.filename or "offer.xlsx"
    if not fname.lower().endswith((".xlsx", ".xls")):
        raise HTTPException(status_code=400, detail="Hanya file Excel (.xlsx/.xls) yang diterima.")

    upload_id = str(uuid4())
    tmp_path = os.path.join(tempfile.gettempdir(), f"{upload_id}_{fname}")
    content = await file.read()
    with open(tmp_path, "wb") as f:
        f.write(content)

    # Insert upload record so front-end immediately shows "processing"
    from src.models.database import SupplierPriceUpload
    from datetime import datetime
    upload_rec = SupplierPriceUpload(
        upload_id=upload_id,
        original_filename=fname,
        supplier_name=supplier_name or "",
        currency=currency,
        status="processing",
        user_id=BOT_USER_ID,
    )
    db.add(upload_rec)
    await db.commit()

    background_tasks.add_task(
        _run_offer_processor, tmp_path, upload_id, fname, supplier_name, currency
    )

    return {
        "upload_id": upload_id,
        "status": "processing",
        "message": f"File '{fname}' diterima dan sedang diproses. Cek status di TRADE → Penawaran Supplier.",
        "supplier_name": supplier_name,
        "currency": currency,
    }

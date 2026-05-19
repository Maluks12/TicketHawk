import { NextRequest, NextResponse } from 'next/server';
import { writeFile, mkdir } from 'fs/promises';
import { join } from 'path';
import { randomUUID } from 'crypto';

const ALLOWED_TYPES: Record<string, string> = {
  'image/jpeg': 'jpg',
  'image/jpg':  'jpg',
  'image/png':  'png',
  'image/webp': 'webp',
  'image/gif':  'gif',
};

const MAX_SIZE = 10 * 1024 * 1024; // 10 MB

/**
 * Local-dev image upload fallback.
 * Stores files under /public/uploads/ and returns an absolute URL.
 *
 * IMPORTANT: Set NEXT_PUBLIC_APP_URL in .env.local to your network IP so
 * images work on mobile devices, e.g.:
 *   NEXT_PUBLIC_APP_URL=http://192.168.1.129:3000
 *
 * Without it, stored URLs use localhost which only works in the browser,
 * not on mobile. In production the backend R2 handler is used instead.
 */
export async function POST(request: NextRequest) {
  try {
    const formData = await request.formData();
    const file = formData.get('image') as File | null;

    if (!file) {
      return NextResponse.json({ error: 'No image provided (field name: image)' }, { status: 400 });
    }

    const mime = file.type || '';
    const ext  = ALLOWED_TYPES[mime] ?? (file.name.split('.').pop() ?? '').toLowerCase();

    if (!Object.values(ALLOWED_TYPES).includes(ext)) {
      return NextResponse.json(
        { error: 'Unsupported file type — use JPEG, PNG, WebP or GIF' },
        { status: 400 },
      );
    }

    if (file.size > MAX_SIZE) {
      return NextResponse.json({ error: 'File too large — max 10 MB' }, { status: 400 });
    }

    const bytes    = await file.arrayBuffer();
    const buffer   = Buffer.from(bytes);
    const filename = `${randomUUID()}.${ext}`;
    const dir      = join(process.cwd(), 'public', 'uploads');

    await mkdir(dir, { recursive: true });
    await writeFile(join(dir, filename), buffer);

    // Prefer NEXT_PUBLIC_APP_URL (network-accessible) over the request host.
    // This ensures the stored URL works on mobile devices that can't reach localhost.
    const appUrl  = process.env.NEXT_PUBLIC_APP_URL?.replace(/\/$/, '');
    const host    = request.headers.get('host') ?? 'localhost:3000';
    const proto   = host.startsWith('localhost') || /^\d+\.\d+\.\d+\.\d+/.test(host) ? 'http' : 'https';
    const baseUrl = appUrl || `${proto}://${host}`;
    const imageUrl = `${baseUrl}/uploads/${filename}`;

    return NextResponse.json({ image_url: imageUrl });
  } catch (err) {
    console.error('[/api/upload]', err);
    return NextResponse.json({ error: 'Local upload failed' }, { status: 500 });
  }
}
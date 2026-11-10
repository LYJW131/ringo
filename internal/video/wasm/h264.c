// A small face on libavcodec's H.264 decoder, for internal/video to call
// through wazero. It decodes a sample at a time, as MP4 holds them, and
// turns each picture into RGBA here, where the loop is compiled with SIMD,
// rather than in Go.
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include "libavcodec/avcodec.h"

typedef struct {
  AVCodecContext *c;
  AVPacket *pkt;
  AVFrame *frame;
  uint8_t *rgba;
  int rgbaSize;
} dec;

__attribute__((export_name("h264_malloc"))) void *h264_malloc(int n) { return malloc(n); }
__attribute__((export_name("h264_free"))) void h264_free(void *p) { free(p); }

// h264_open makes a decoder of the stream an avcC record describes.
__attribute__((export_name("h264_open"))) dec *h264_open(const uint8_t *avcc, int n) {
  const AVCodec *codec = avcodec_find_decoder(AV_CODEC_ID_H264);
  if (!codec) return NULL;
  dec *d = calloc(1, sizeof(dec));
  d->c = avcodec_alloc_context3(codec);
  d->c->extradata = av_mallocz(n + AV_INPUT_BUFFER_PADDING_SIZE);
  memcpy(d->c->extradata, avcc, n);
  d->c->extradata_size = n;
  d->c->thread_count = 1;
  if (avcodec_open2(d->c, codec, NULL) < 0) {
    avcodec_free_context(&d->c);
    free(d);
    return NULL;
  }
  d->pkt = av_packet_alloc();
  d->frame = av_frame_alloc();
  return d;
}

// h264_send gives the decoder one sample, its NAL units length-prefixed as
// MP4 holds them; none (n == 0) drains it of the pictures it holds back.
__attribute__((export_name("h264_send"))) int h264_send(dec *d, uint8_t *data, int n) {
  if (n == 0) return avcodec_send_packet(d->c, NULL);
  av_packet_unref(d->pkt);
  if (av_new_packet(d->pkt, n) < 0) return -1;
  memcpy(d->pkt->data, data, n);
  return avcodec_send_packet(d->c, d->pkt);
}

// h264_receive answers 1 with a picture, 0 when the decoder wants another
// sample, 2 once drained, and below 0 for an error.
__attribute__((export_name("h264_receive"))) int h264_receive(dec *d) {
  int r = avcodec_receive_frame(d->c, d->frame);
  if (r == 0) return 1;
  if (r == AVERROR(EAGAIN)) return 0;
  if (r == AVERROR_EOF) return 2;
  return r < 0 ? r : -1;
}

// h264_flush forgets what the decoder holds, to start again from the first
// sample.
__attribute__((export_name("h264_flush"))) void h264_flush(dec *d) { avcodec_flush_buffers(d->c); }

static inline uint8_t clamp(int v) { return v < 0 ? 0 : v > 255 ? 255 : v; }

// h264_rgba turns the picture received last into RGBA, answering where it
// is, or 0 when it isn't 8-bit 4:2:0. out gets its width and height. The
// matrix and range are the stream's: Apple's are BT.709 in TV range, and
// reading them as full-range BT.601, as image.YCbCr does, would wash them
// out against the still cover they fade in over.
__attribute__((export_name("h264_rgba"))) uint8_t *h264_rgba(dec *d, int32_t *out) {
  AVFrame *f = d->frame;
  if (f->format != AV_PIX_FMT_YUV420P && f->format != AV_PIX_FMT_YUVJ420P) return 0;
  int w = f->width, h = f->height;
  int size = w * h * 4;
  if (size > d->rgbaSize) {
    free(d->rgba);
    d->rgba = malloc(size);
    if (!d->rgba) {
      d->rgbaSize = 0;
      return 0;
    }
    d->rgbaSize = size;
  }
  int full = f->color_range == AVCOL_RANGE_JPEG || f->format == AV_PIX_FMT_YUVJ420P;
  int bt601 = f->colorspace == AVCOL_SPC_BT470BG || f->colorspace == AVCOL_SPC_SMPTE170M ||
              (f->colorspace == AVCOL_SPC_UNSPECIFIED && h < 720);
  // Coefficients times 65536: luma's scale, then Cr to red, Cb and Cr to
  // green, and Cb to blue.
  int ky, kr, kgb, kgr, kb, y0;
  if (bt601) {
    if (full) { ky = 65536; kr = 91881; kgb = 22554; kgr = 46802; kb = 116130; }
    else { ky = 76309; kr = 104597; kgb = 25675; kgr = 53279; kb = 132201; }
  } else {
    if (full) { ky = 65536; kr = 103206; kgb = 12276; kgr = 30679; kb = 121609; }
    else { ky = 76309; kr = 117489; kgb = 13975; kgr = 34925; kb = 138438; }
  }
  y0 = full ? 0 : 16;
  for (int j = 0; j < h; j++) {
    const uint8_t *yp = f->data[0] + j * f->linesize[0];
    const uint8_t *up = f->data[1] + (j >> 1) * f->linesize[1];
    const uint8_t *vp = f->data[2] + (j >> 1) * f->linesize[2];
    uint8_t *o = d->rgba + j * w * 4;
    for (int i = 0; i < w; i++) {
      int y = (yp[i] - y0) * ky + 32768;
      int u = up[i >> 1] - 128, v = vp[i >> 1] - 128;
      o[0] = clamp((y + kr * v) >> 16);
      o[1] = clamp((y - kgb * u - kgr * v) >> 16);
      o[2] = clamp((y + kb * u) >> 16);
      o[3] = 255;
      o += 4;
    }
  }
  out[0] = w;
  out[1] = h;
  return d->rgba;
}

__attribute__((export_name("h264_close"))) void h264_close(dec *d) {
  av_frame_free(&d->frame);
  av_packet_free(&d->pkt);
  avcodec_free_context(&d->c);
  free(d->rgba);
  free(d);
}

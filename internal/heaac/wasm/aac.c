// A small face on libavcodec's AAC decoder, for internal/heaac to call
// through wazero. It decodes one packet at a time, as MP4 holds them, to
// interleaved stereo float: what MyGo's MP4 decoder wants of a codec.
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include "libavcodec/avcodec.h"

typedef struct {
  AVCodecContext *c;
  AVPacket *pkt;
  AVFrame *frame;
  float *out;
  int outFrames;
} dec;

__attribute__((export_name("aac_malloc"))) void *aac_malloc(int n) { return malloc(n); }
__attribute__((export_name("aac_free"))) void aac_free(void *p) { free(p); }

// aac_open makes a decoder of the stream an AudioSpecificConfig describes.
__attribute__((export_name("aac_open"))) dec *aac_open(const uint8_t *asc, int n) {
  const AVCodec *codec = avcodec_find_decoder(AV_CODEC_ID_AAC);
  if (!codec) return NULL;
  dec *d = calloc(1, sizeof(dec));
  d->c = avcodec_alloc_context3(codec);
  d->c->extradata = av_mallocz(n + AV_INPUT_BUFFER_PADDING_SIZE);
  memcpy(d->c->extradata, asc, n);
  d->c->extradata_size = n;
  if (avcodec_open2(d->c, codec, NULL) < 0) {
    avcodec_free_context(&d->c);
    free(d);
    return NULL;
  }
  d->pkt = av_packet_alloc();
  d->frame = av_frame_alloc();
  return d;
}

// put appends a frame to out as stereo: a mono one in both, and of more
// channels, the first two, which are the front left and right of the
// stereo and parametric-stereo streams this is for.
static int put(dec *d, int at) {
  AVFrame *f = d->frame;
  int ch = f->ch_layout.nb_channels;
  if (f->format != AV_SAMPLE_FMT_FLTP || ch < 1) return -1;
  int need = at + f->nb_samples;
  if (need > d->outFrames) {
    float *grown = realloc(d->out, need * 2 * sizeof(float));
    if (!grown) return -1;
    d->out = grown;
    d->outFrames = need;
  }
  const float *l = (const float *)f->extended_data[0];
  const float *r = (const float *)f->extended_data[ch > 1 ? 1 : 0];
  float *o = d->out + at * 2;
  for (int i = 0; i < f->nb_samples; i++) {
    o[2 * i] = l[i];
    o[2 * i + 1] = r[i];
  }
  return f->nb_samples;
}

// aac_decode decodes one packet, answering how many stereo frames it gave,
// at aac_out, or below 0 for an error.
__attribute__((export_name("aac_decode"))) int aac_decode(dec *d, uint8_t *data, int n) {
  av_packet_unref(d->pkt);
  if (av_new_packet(d->pkt, n) < 0) return -1;
  memcpy(d->pkt->data, data, n);
  int r = avcodec_send_packet(d->c, d->pkt);
  if (r < 0) return r;
  int total = 0;
  for (;;) {
    r = avcodec_receive_frame(d->c, d->frame);
    if (r == AVERROR(EAGAIN) || r == AVERROR_EOF) break;
    if (r < 0) return r;
    int got = put(d, total);
    if (got < 0) return -1;
    total += got;
  }
  return total;
}

__attribute__((export_name("aac_out"))) float *aac_out(dec *d) { return d->out; }

// aac_flush forgets what the decoder holds, as after a seek.
__attribute__((export_name("aac_flush"))) void aac_flush(dec *d) { avcodec_flush_buffers(d->c); }

__attribute__((export_name("aac_close"))) void aac_close(dec *d) {
  av_frame_free(&d->frame);
  av_packet_free(&d->pkt);
  avcodec_free_context(&d->c);
  free(d->out);
  free(d);
}

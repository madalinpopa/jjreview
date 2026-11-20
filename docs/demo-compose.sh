#!/bin/sh
set -eu

if [ $# -ne 3 ]; then
	echo "usage: $0 <raw.gif> <out.gif> <tint opacity>" >&2
	exit 2
fi

raw=$1
out=$2
tint=$3
margin=40
radius=12

size=$(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=s=x:p=0 "$raw")
w=${size%x*}
h=${size#*x}
cw=$((w + 2 * margin))
ch=$((h + 2 * margin))

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

bg=0x$(ffmpeg -v error -i "$raw" -vf "crop=1:1:$((w - 2)):$((h - 2))" -frames:v 1 -f rawvideo -pix_fmt rgb24 - | od -An -tx1 | tr -d ' \n')

ffmpeg -v error -f lavfi -i "gradients=s=${cw}x${ch}:n=5:c0=0x2a1b5c:c1=0x7b5cff:c2=0xff6f61:c3=0xffb347:c4=0x1fb5ff:x0=0:y0=${ch}:x1=${cw}:y1=0:type=linear:speed=0,gblur=sigma=60" \
	-frames:v 1 "$tmp/wall.png"

ffmpeg -v error -f lavfi -i "color=c=black:s=${w}x${h},format=gray" -vf \
	"geq=lum='255*lte(hypot(max(0,max(${radius}-X,X-(W-1-${radius}))),max(0,max(${radius}-Y,Y-(H-1-${radius})))),${radius})'" \
	-frames:v 1 "$tmp/mask.png"

ffmpeg -v error -i "$tmp/wall.png" -i "$tmp/mask.png" -filter_complex \
	"[0]split[wall][src];
	[src]crop=${w}:${h}:${margin}:${margin},gblur=sigma=24,format=rgba[blur];
	color=c=${bg}@${tint}:s=${w}x${h},format=rgba[tint];
	[blur][tint]overlay,format=rgb24[glass];
	[glass][1]alphamerge[win];
	[wall][win]overlay=${margin}:${margin}" \
	-frames:v 1 "$tmp/base.png"

r=$((0x$(printf '%s' "$bg" | cut -c3-4)))
g=$((0x$(printf '%s' "$bg" | cut -c5-6)))
b=$((0x$(printf '%s' "$bg" | cut -c7-8)))
lift() { echo "A+(B-$1)*(1-min(1,abs(A-$1)/96))"; }

graph="[0]format=gbrp[fg];
	[1]split[base][src];
	[src]crop=${w}:${h}:${margin}:${margin},format=gbrp[glass];
	[fg][glass]blend=c0_expr='$(lift "$g")':c1_expr='$(lift "$b")':c2_expr='$(lift "$r")':shortest=1,format=rgb24[win];
	[base][win]overlay=${margin}:${margin}:shortest=1"

ffmpeg -v error -i "$raw" -loop 1 -i "$tmp/base.png" -filter_complex "$graph,palettegen=max_colors=256:stats_mode=full" \
	-update 1 "$tmp/palette.png"
ffmpeg -v error -y -i "$raw" -loop 1 -i "$tmp/base.png" -i "$tmp/palette.png" -filter_complex \
	"$graph[v];[v][2]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle" \
	"$out"

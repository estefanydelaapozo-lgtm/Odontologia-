import { useRef, useState, type PointerEvent } from 'react'

export default function SignaturePad({label='Firma del paciente',onChange}:{label?:string;onChange:(signature:string)=>void}) {
 const canvas=useRef<HTMLCanvasElement>(null),[drawing,setDrawing]=useState(false),[hasInk,setHasInk]=useState(false)
 const point=(e:PointerEvent<HTMLCanvasElement>)=>{const el=canvas.current!,rect=el.getBoundingClientRect();return{x:(e.clientX-rect.left)*el.width/rect.width,y:(e.clientY-rect.top)*el.height/rect.height}}
 const start=(e:PointerEvent<HTMLCanvasElement>)=>{const el=canvas.current!,ctx=el.getContext('2d');if(!ctx)return;el.setPointerCapture(e.pointerId);const p=point(e);ctx.fillStyle='#194b45';ctx.beginPath();ctx.arc(p.x,p.y,1.3,0,Math.PI*2);ctx.fill();ctx.beginPath();ctx.moveTo(p.x,p.y);ctx.lineWidth=2.4;ctx.lineCap='round';ctx.lineJoin='round';ctx.strokeStyle='#194b45';setDrawing(true);setHasInk(true)}
 const move=(e:PointerEvent<HTMLCanvasElement>)=>{if(!drawing)return;const ctx=canvas.current?.getContext('2d');if(!ctx)return;const p=point(e);ctx.lineTo(p.x,p.y);ctx.stroke()}
 const finish=()=>{if(!drawing)return;setDrawing(false);const data=canvas.current?.toDataURL('image/png')??'';onChange(data)}
 const clear=()=>{const el=canvas.current;el?.getContext('2d')?.clearRect(0,0,el.width,el.height);setHasInk(false);onChange('')}
 return <div className="signature-pad"><label>{label}</label><canvas ref={canvas} width={640} height={150} onPointerDown={start} onPointerMove={move} onPointerUp={finish} onPointerCancel={finish} aria-label={label}/><div><small>Firma con el dedo, lápiz digital o mouse.</small><button type="button" className="text-button" onClick={clear} disabled={!hasInk}>Borrar firma</button></div></div>
}

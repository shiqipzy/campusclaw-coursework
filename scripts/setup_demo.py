"""Idempotently bind the existing built-in tutor to a real mathematics handout and MCP tools."""
import sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'apitest'))
from support import Context

def main():
    c=Context('http://127.0.0.1:8080','campusclaw')
    auth=c.http('POST','/api/login',{'username':'teacher_a','password':c.env['TEACHER_A_PASSWORD']},who=None)
    c.tokens['teacher_a']=auth['token']
    server=next(s for s in c.http('GET','/api/mcp/servers') if s['name']=='campus-tools')
    c.http('POST',f'/api/mcp/servers/{server["id"]}/connect')
    materials=c.http('GET','/api/materials')
    material=next((m for m in materials if m['title']=='数学自学笔记'),None)
    if material is None:
        material=c.http('POST','/api/materials',form=('数学自学笔记.md','数学自学笔记\n去括号：括号前的系数应乘到括号内每一项，遇到负号要逐项检查符号。\n解方程：移项时改变符号，合并同类项后再检查等式两边。\n单位换算：同一物理维度才可以换算。1m=100cm，1kg=1000g。'),expected=201)
    skill=next(s for s in c.http('GET','/api/skills') if s['name']=='数学解题引导')
    source=skill['source_text']
    if 'formula_hint@campus-tools' not in source:source=source.replace('工具：knowledge_search','工具：knowledge_search,formula_hint@campus-tools,unit_convert@campus-tools')
    c.http('PUT',f'/api/skills/{skill["id"]}',{'source_text':source})
    agents=c.http('GET','/api/agents')
    for agent in agents:
        if agent['name'] not in ['解题助手','作业诊断助手']:continue
        body={k:agent[k] for k in ['name','prompt','enabled','skill_ids','material_ids']}
        body['material_ids']=sorted(set(body['material_ids']+[material['id']]))
        c.http('PUT',f'/api/agents/{agent["id"]}',body)
    print('PASS: existing main CampusClaw tutor, handout, built-in skill and real MCP tools ready on localhost:8080')
if __name__=='__main__':main()

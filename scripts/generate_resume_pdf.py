import json
import sys
import os
from reportlab.lib.pagesizes import letter
from reportlab.lib import colors
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.platypus import SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle

def generate_elements(profile, base_font_size, leading, spacer_height):
    styles = getSampleStyleSheet()
    
    # Proportional font sizes based on base_font_size
    name_size = base_font_size + 8.5
    contact_size = base_font_size
    section_size = base_font_size + 2.0
    item_header_size = base_font_size + 0.5
    body_size = base_font_size
    
    # Custom styles
    name_style = ParagraphStyle(
        'ResumeName',
        parent=styles['Normal'],
        fontName='Times-Bold',
        fontSize=name_size,
        leading=name_size * 1.25,
        alignment=1, # Center
        spaceAfter=3
    )
    
    contact_style = ParagraphStyle(
        'ResumeContact',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=contact_size,
        leading=contact_size * 1.2,
        alignment=1, # Center
        spaceAfter=8
    )
    
    section_title_style = ParagraphStyle(
        'ResumeSectionTitle',
        parent=styles['Normal'],
        fontName='Times-Bold',
        fontSize=section_size,
        leading=section_size * 1.2,
        spaceBefore=6,
        spaceAfter=1,
        textColor=colors.HexColor("#1A365D") # Dark Blue
    )
    
    item_header_style = ParagraphStyle(
        'ResumeItemHeader',
        parent=styles['Normal'],
        fontName='Times-Bold',
        fontSize=item_header_size,
        leading=item_header_size * 1.2,
        spaceAfter=1
    )
    
    item_subheader_style = ParagraphStyle(
        'ResumeItemSubheader',
        parent=styles['Normal'],
        fontName='Times-Italic',
        fontSize=body_size,
        leading=leading,
        spaceAfter=1
    )
    
    body_style = ParagraphStyle(
        'ResumeBody',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=body_size,
        leading=leading,
        spaceAfter=1
    )
    
    bullet_style = ParagraphStyle(
        'ResumeBullet',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=body_size,
        leading=leading,
        leftIndent=12,
        firstLineIndent=-8,
        spaceAfter=1.5
    )

    elements = []
    
    # 1. Header
    info = profile.get("personal_info", {})
    elements.append(Paragraph(info.get("name", ""), name_style))
    
    contact_parts = []
    if info.get("location"): contact_parts.append(info["location"])
    if info.get("email"): contact_parts.append(info["email"])
    if info.get("phone"): contact_parts.append(info["phone"])
    if info.get("linkedin"): contact_parts.append(info["linkedin"])
    if info.get("website"): contact_parts.append(info["website"])
    
    contact_str = "  |  ".join(contact_parts)
    elements.append(Paragraph(contact_str, contact_style))
    
    # Helper for horizontal lines
    def draw_section_line(title):
        elements.append(Paragraph(title.upper(), section_title_style))
        # Use a table to draw a solid thin border line
        line_table = Table([[""]], colWidths=[540], rowHeights=[1])
        line_table.setStyle(TableStyle([
            ('LINEBELOW', (0,0), (-1,-1), 0.75, colors.HexColor("#A0AEC0")),
            ('BOTTOMPADDING', (0,0), (-1,-1), 0),
            ('TOPPADDING', (0,0), (-1,-1), 0),
        ]))
        elements.append(line_table)
        elements.append(Spacer(1, spacer_height))
        
    # 2. Experience Section
    experience = profile.get("experience", [])
    if experience:
        draw_section_line("Professional Experience")
        for exp in experience:
            company = exp.get("company", "")
            dates = f"{exp.get('start_date', '')} - {exp.get('end_date', '')}"
            role = exp.get("role", "")
            loc = exp.get("location", "")

            # Line 1: Company (Left) and Location (Right)
            company_para = Paragraph(f"<b>{company}</b>", item_header_style)
            loc_para = Paragraph(loc, ParagraphStyle('RightLoc', parent=body_style, alignment=2))
            
            line1_table = Table([[company_para, loc_para]], colWidths=[380, 160])
            line1_table.setStyle(TableStyle([
                ('VALIGN', (0,0), (-1,-1), 'BOTTOM'),
                ('LEFTPADDING', (0,0), (-1,-1), 0),
                ('RIGHTPADDING', (0,0), (-1,-1), 0),
                ('BOTTOMPADDING', (0,0), (-1,-1), 0),
                ('TOPPADDING', (0,0), (-1,-1), 1),
            ]))
            elements.append(line1_table)
            
            # Line 2: Role (Left) and Dates (Right)
            role_para = Paragraph(f"<i>{role}</i>", item_subheader_style)
            dates_para = Paragraph(dates, ParagraphStyle('RightDate', parent=item_subheader_style, alignment=2))
            
            line2_table = Table([[role_para, dates_para]], colWidths=[380, 160])
            line2_table.setStyle(TableStyle([
                ('VALIGN', (0,0), (-1,-1), 'TOP'),
                ('LEFTPADDING', (0,0), (-1,-1), 0),
                ('RIGHTPADDING', (0,0), (-1,-1), 0),
                ('BOTTOMPADDING', (0,0), (-1,-1), 1),
                ('TOPPADDING', (0,0), (-1,-1), 0),
            ]))
            elements.append(line2_table)
                
            for bullet in exp.get("bullets", []):
                elements.append(Paragraph(f"&bull;&nbsp;&nbsp;{bullet}", bullet_style))
            elements.append(Spacer(1, spacer_height))
            
    # 3. Projects Section
    projects = profile.get("projects", [])
    if projects:
        draw_section_line("Projects")
        for proj in projects:
            name = proj.get("name", "")
            tech = f"({', '.join(proj.get('technologies', []))})" if proj.get('technologies') else ""
            desc = proj.get("description", "")
            details = proj.get("details", "")
            
            proj_title = f"<b>{name}</b> {tech}" if tech else f"<b>{name}</b>"
            elements.append(Paragraph(proj_title, item_header_style))
            if desc:
                elements.append(Paragraph(desc, body_style))
            if details:
                elements.append(Paragraph(f"&bull;&nbsp;&nbsp;{details}", bullet_style))
            elements.append(Spacer(1, spacer_height))

    # 4. Education Section
    education = profile.get("education", [])
    if education:
        draw_section_line("Education")
        for edu in education:
            inst = edu.get("institution", "")
            deg = f"{edu.get('degree', '')} in {edu.get('major', '')}"
            dates = edu.get("graduation_date", "")
            loc = edu.get("location", "")
            gpa = f"GPA: {edu.get('gpa')}" if edu.get('gpa') else ""
            
            edu_header_text = f"<b>{inst}</b>"
            if loc:
                edu_header_text += f" - {loc}"
                
            edu_table = Table(
                [[Paragraph(edu_header_text, item_header_style), 
                  Paragraph(dates, ParagraphStyle('RightEduDate', parent=item_header_style, alignment=2))]],
                colWidths=[400, 140]
            )
            edu_table.setStyle(TableStyle([
                ('VALIGN', (0,0), (-1,-1), 'BOTTOM'),
                ('LEFTPADDING', (0,0), (-1,-1), 0),
                ('RIGHTPADDING', (0,0), (-1,-1), 0),
                ('BOTTOMPADDING', (0,0), (-1,-1), 0),
                ('TOPPADDING', (0,0), (-1,-1), 1),
            ]))
            elements.append(edu_table)
            
            deg_text = deg
            if gpa:
                deg_text += f" ({gpa})"
            elements.append(Paragraph(deg_text, item_subheader_style))
            
            if edu.get("details"):
                elements.append(Paragraph(edu.get("details"), body_style))
            elements.append(Spacer(1, spacer_height))
            
    # 5. Skills Section
    skills = profile.get("skills", {})
    if skills:
        draw_section_line("Skills")
        skills_lines = []
        for cat, items in skills.items():
            cat_name = cat.replace("_", " ").title()
            skills_lines.append(f"<b>{cat_name}:</b> {', '.join(items)}")
            
        for line in skills_lines:
            elements.append(Paragraph(line, body_style))
            
    return elements

def build_resume_pdf(profile_path, output_path):
    try:
        with open(profile_path, 'r', encoding='utf-8') as f:
            profile = json.load(f)
    except Exception as e:
        print(f"Error loading profile: {str(e)}")
        sys.exit(1)
        
    # Visual locking system: Try multiple presets to keep content on exactly 1 page
    presets = [
        {"base_font_size": 9.5, "leading": 12.5, "margin": 36, "spacer": 2.0},  # Standard layout
        {"base_font_size": 9.0, "leading": 11.5, "margin": 32, "spacer": 1.5},  # Compact
        {"base_font_size": 8.5, "leading": 10.5, "margin": 28, "spacer": 1.0},  # Highly compressed
    ]
    
    success = False
    for idx, pr in enumerate(presets):
        doc = SimpleDocTemplate(
            output_path, 
            pagesize=letter,
            rightMargin=pr["margin"],
            leftMargin=pr["margin"],
            topMargin=pr["margin"],
            bottomMargin=pr["margin"]
        )
        
        elements = generate_elements(profile, pr["base_font_size"], pr["leading"], pr["spacer"])
        doc.build(elements)
        
        if doc.page == 1:
            print(f"Successfully generated 1-page resume PDF at: {output_path} (preset {idx+1})")
            success = True
            break
            
    if not success:
        print(f"Warning: Resume exceeded 1 page ({doc.page} pages) even with maximum visual lock compression.")
        
if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: python generate_resume_pdf.py <profile.json> <output.pdf>")
        sys.exit(1)
    build_resume_pdf(sys.argv[1], sys.argv[2])

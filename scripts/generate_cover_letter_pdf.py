import sys
import os
import json
from reportlab.lib.pagesizes import letter
from reportlab.pdfgen import canvas
from reportlab.lib import colors
from reportlab.lib.units import inch
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.platypus import SimpleDocTemplate, Paragraph, Spacer

def create_cover_letter_pdf(profile_path, draft_text_path, output_path):
    try:
        with open(profile_path, 'r', encoding='utf-8') as f:
            profile = json.load(f)
    except Exception as e:
        print(f"Error loading profile: {str(e)}")
        sys.exit(1)
        
    try:
        with open(draft_text_path, 'r', encoding='utf-8') as f:
            draft_text = f.read()
    except Exception as e:
        print(f"Error loading draft text: {str(e)}")
        sys.exit(1)
        
    doc = SimpleDocTemplate(output_path, pagesize=letter,
                            rightMargin=inch, leftMargin=inch,
                            topMargin=inch, bottomMargin=inch)
    
    styles = getSampleStyleSheet()
    
    # Header Styles
    header_name_style = ParagraphStyle(
        'HeaderName',
        parent=styles['Normal'],
        fontName='Times-Bold',
        fontSize=12,
        alignment=1, # Center
        spaceAfter=0
    )
    
    header_info_style = ParagraphStyle(
        'HeaderInfo',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=11,
        alignment=1, # Center
        spaceAfter=12
    )
    
    # Body Style (Times-Roman, Double/1.5 spaced)
    body_style = ParagraphStyle(
        'BodyStyle',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=11,
        leading=18, # Clean line height
        alignment=0, # Left
        spaceAfter=10
    )
    
    elements = []
    
    # Parse User Info for Header
    info = profile.get("personal_info", {})
    name = info.get("name", "Applicant Name")
    
    contact_parts = []
    if info.get("email"): contact_parts.append(info["email"])
    if info.get("phone"): contact_parts.append(info["phone"])
    if info.get("location"): contact_parts.append(info["location"])
    
    contact_str = " | ".join(contact_parts)
    
    # Add Header
    elements.append(Paragraph(name, header_name_style))
    elements.append(Paragraph(contact_str, header_info_style))
    elements.append(Spacer(1, 0.2*inch))
    
    # Add Body Content (split by paragraph/empty lines)
    lines = draft_text.split('\n')
    current_para = ""
    for line in lines:
        stripped = line.strip()
        if stripped == "":
            if current_para:
                elements.append(Paragraph(current_para, body_style))
                current_para = ""
        else:
            if current_para:
                current_para += " " + stripped
            else:
                current_para = stripped
                
    if current_para:
        elements.append(Paragraph(current_para, body_style))
        
    doc.build(elements)
    print(f"Successfully generated Cover Letter PDF: {output_path}")

if __name__ == "__main__":
    if len(sys.argv) < 4:
        print("Usage: python generate_cover_letter_pdf.py <profile_json> <draft_text_file> <output_pdf>")
        sys.exit(1)
        
    profile_json = sys.argv[1]
    draft_text_file = sys.argv[2]
    output_pdf = sys.argv[3]
    
    create_cover_letter_pdf(profile_json, draft_text_file, output_pdf)
